package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Lutefd/challenge-bravo/internal/cache"
	"github.com/Lutefd/challenge-bravo/internal/commons"
	"github.com/Lutefd/challenge-bravo/internal/logger"
	"github.com/Lutefd/challenge-bravo/internal/model"
	"github.com/Lutefd/challenge-bravo/internal/repository"
)

type RateUpdater struct {
	repo        repository.CurrencyRepository
	cache       cache.Cache
	externalAPI ExternalAPIClient
	interval    time.Duration
}

func NewRateUpdater(repo repository.CurrencyRepository, cache cache.Cache, externalAPI ExternalAPIClient, interval time.Duration) *RateUpdater {
	return &RateUpdater{
		repo:        repo,
		cache:       cache,
		externalAPI: externalAPI,
		interval:    interval,
	}
}

func (ru *RateUpdater) Start(ctx context.Context) {
	ticker := time.NewTicker(ru.interval)
	if err := ru.populateRates(ctx); err != nil {
		logger.Errorf("error updating rates on startup: %v", err)
	}
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("rate updater stopped")
			return
		case <-ticker.C:
			if err := ru.updateRates(ctx); err != nil {
				logger.Errorf("error updating rates: %v", err)
			}
		}
	}
}

func (ru *RateUpdater) updateRates(ctx context.Context) error {
	rates, err := ru.externalAPI.FetchRates(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch rates: %w", err)
	}

	for code, rate := range rates.Rates {
		currency := &model.Currency{
			Code:      code,
			Rate:      rate,
			UpdatedAt: time.Unix(rates.Timestamp, 0),
		}

		if err := ru.repo.Update(ctx, currency); err != nil {
			logger.Errorf("failed to update currency %s in repository: %v", code, err)
		}

		if err := ru.cache.Set(ctx, code, rate, 1*time.Hour); err != nil {
			logger.Errorf("failed to update currency %s in cache: %v", code, err)
		}
	}

	log.Println("rates updated successfully")
	return nil
}

func (ru *RateUpdater) populateRates(ctx context.Context) error {
	rates, err := ru.externalAPI.FetchRates(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch rates: %w", err)
	}

	for code, rate := range rates.Rates {
		currency := &model.Currency{
			Code:      code,
			Rate:      rate,
			UpdatedAt: time.Unix(rates.Timestamp, 0),
		}
		_, err := ru.repo.GetByCode(ctx, code)
		if err != nil {
			if err.Error() == "currency not found" {
				err = ru.repo.Create(ctx, currency)
				if err != nil {
					logger.Errorf("failed to create currency %s in repository: %v", code, err)
					continue
				}
			} else {
				logger.Errorf("failed to get currency %s in repository: %v", code, err)
				continue
			}
		} else {
			err = ru.repo.Update(ctx, currency)
			if err != nil {
				logger.Errorf("failed to update currency %s in repository: %v", code, err)
				continue
			}
		}
		if err := ru.cache.Set(ctx, code, rate, commons.RateUpdaterCacheExipiration); err != nil {
			logger.Errorf("failed to update currency %s in cache: %v", code, err)
		}
	}

	log.Println("rates updated successfully")
	return nil
}
