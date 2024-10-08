package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Lutefd/challenge-bravo/internal/commons"
	"github.com/Lutefd/challenge-bravo/internal/model"
	"github.com/Lutefd/challenge-bravo/internal/service"
	"github.com/go-chi/chi/v5"
)

type CurrencyHandler struct {
	currencyService service.CurrencyServiceInterface
}

func NewCurrencyHandler(currencyService service.CurrencyServiceInterface) *CurrencyHandler {
	return &CurrencyHandler{
		currencyService: currencyService,
	}
}

func (h *CurrencyHandler) ConvertCurrency(w http.ResponseWriter, r *http.Request) {
	from := strings.ToUpper(r.URL.Query().Get("from"))
	to := strings.ToUpper(r.URL.Query().Get("to"))
	amountStr := r.URL.Query().Get("amount")

	if from == "" || to == "" || (amountStr == "") {
		commons.RespondWithError(w, http.StatusBadRequest, "missing required parameters")
		return
	}
	if len(from) > commons.AllowedCurrencyLength || len(to) > commons.AllowedCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid currency code, must be up to %d characters", commons.AllowedCurrencyLength))
		return
	}
	if len(from) < commons.MinimumCurrencyLength || len(to) < commons.MinimumCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid currency code, must be at least %d characters", commons.MinimumCurrencyLength))
		return
	}

	amount, err := parseAmount(amountStr)
	if err != nil {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid amount")
		return
	}

	if amount < 0 {
		commons.RespondWithError(w, http.StatusBadRequest, "amount must be non-negative")
		return
	}

	result, err := h.currencyService.Convert(r.Context(), from, to, amount)
	if err != nil {
		if errors.Is(err, model.ErrCurrencyNotFound) {
			commons.RespondWithError(w, http.StatusNotFound, err.Error())
		} else {
			commons.RespondWithError(w, http.StatusInternalServerError, "conversion failed")
		}
		return
	}

	commons.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"from":   from,
		"to":     to,
		"amount": amount,
		"result": result,
	})
}

func (h *CurrencyHandler) AddCurrency(w http.ResponseWriter, r *http.Request) {
	var currency struct {
		Code string      `json:"code"`
		Rate interface{} `json:"rate_to_usd"`
	}

	if err := json.NewDecoder(r.Body).Decode(&currency); err != nil {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid request payload")
		return
	}
	if currency.Code == "" {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid currency code")
		return
	}
	if len(currency.Code) > commons.AllowedCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid currency code, must be up to %d characters", commons.AllowedCurrencyLength))
		return
	}

	if len(currency.Code) < commons.MinimumCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid currency code, must be at least %d characters", commons.MinimumCurrencyLength))
		return
	}

	user, ok := r.Context().Value("user").(model.User)
	if !ok {
		commons.RespondWithError(w, http.StatusInternalServerError, "user information not available")
		return
	}
	rate, err := parseRate(currency.Rate)
	if err != nil {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid rate: "+err.Error())
		return
	}

	if rate <= 0 {
		commons.RespondWithError(w, http.StatusBadRequest, "rate must be positive")
		return
	}
	newCurrency := &model.Currency{
		Code:      strings.ToUpper(currency.Code),
		Rate:      rate,
		CreatedBy: user.ID,
		UpdatedBy: user.ID,
		UpdatedAt: time.Now(),
		CreatedAt: time.Now(),
	}

	if err := h.currencyService.AddCurrency(r.Context(), newCurrency); err != nil {
		commons.RespondWithError(w, http.StatusInternalServerError, "failed to add currency")
		return
	}

	commons.RespondWithJSON(w, http.StatusCreated, map[string]string{"message": "currency added successfully"})
}

func (h *CurrencyHandler) UpdateCurrency(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(chi.URLParam(r, "code"))
	if code == "" || len(code) > commons.AllowedCurrencyLength || len(code) < commons.MinimumCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid currency code")
		return
	}

	var input struct {
		Rate interface{} `json:"rate_to_usd"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid request payload")
		return
	}

	rate, err := parseRate(input.Rate)
	if err != nil {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid rate: "+err.Error())
		return
	}

	if rate <= 0 {
		commons.RespondWithError(w, http.StatusBadRequest, "rate must be positive")
		return
	}

	user, ok := r.Context().Value("user").(model.User)
	if !ok {
		commons.RespondWithError(w, http.StatusInternalServerError, "user information not available")
		return
	}

	if err := h.currencyService.UpdateCurrency(r.Context(), code, rate, user.ID); err != nil {
		if err == model.ErrCurrencyNotFound {
			commons.RespondWithError(w, http.StatusNotFound, "currency not found")
		} else {
			commons.RespondWithError(w, http.StatusInternalServerError, "failed to update currency")
		}
		return
	}

	commons.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "currency updated successfully"})
}

func (h *CurrencyHandler) RemoveCurrency(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(chi.URLParam(r, "code"))

	if code == "" {
		commons.RespondWithError(w, http.StatusBadRequest, "invalid currency code")
		return
	}
	if len(code) > commons.AllowedCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid currency code, must be up to %d characters", commons.AllowedCurrencyLength))
		return
	}
	if len(code) < commons.MinimumCurrencyLength {
		commons.RespondWithError(w, http.StatusBadRequest, fmt.Sprintf("invalid currency code, must be at least %d characters", commons.MinimumCurrencyLength))
		return
	}
	if err := h.currencyService.RemoveCurrency(r.Context(), code); err != nil {
		commons.RespondWithError(w, http.StatusInternalServerError, "failed to remove currency")
		return
	}

	commons.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "currency removed successfully"})
}
func parseAmount(amountStr string) (float64, error) {
	amountStr = strings.Replace(amountStr, ",", ".", -1)
	return strconv.ParseFloat(amountStr, 64)
}
func parseRate(rate interface{}) (float64, error) {
	switch v := rate.(type) {
	case float64:
		return v, nil
	case string:
		v = strings.Replace(v, ",", ".", -1)
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported rate type")
	}
}
