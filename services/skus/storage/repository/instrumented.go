package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/brave-intl/bat-go/services/skus/model"
)

// PromIssuer wraps Issuer with Prometheus metrics.
type PromIssuer struct {
	name string
	repo *Issuer
	vec  *prometheus.SummaryVec
}

func NewPromIssuer(name string, repo *Issuer) *PromIssuer {
	result := &PromIssuer{
		name: name,
		repo: repo,
		vec: promauto.NewSummaryVec(prometheus.SummaryOpts{
			Name:       "skus_repository_issuer_duration_seconds",
			Help:       "issuer repository runtime duration and result",
			MaxAge:     time.Minute,
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		}, []string{"instance_name", "method", "result"}),
	}

	return result
}

func (r *PromIssuer) GetByMerchID(ctx context.Context, dbi sqlx.QueryerContext, merchID string) (rt *model.Issuer, err error) {
	now := time.Now()

	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ob := r.vec.WithLabelValues(r.name, "GetByMerchID", result)
		ob.Observe(time.Since(now).Seconds())
	}()

	rt, err = r.repo.GetByMerchID(ctx, dbi, merchID)

	return rt, err
}

func (r *PromIssuer) GetByPubKey(ctx context.Context, dbi sqlx.QueryerContext, pubKey string) (rt *model.Issuer, err error) {
	now := time.Now()

	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ob := r.vec.WithLabelValues(r.name, "GetByPubKey", result)
		ob.Observe(time.Since(now).Seconds())
	}()

	rt, err = r.repo.GetByPubKey(ctx, dbi, pubKey)

	return rt, err
}

func (r *PromIssuer) Create(ctx context.Context, dbi sqlx.QueryerContext, req model.IssuerNew) (rt *model.Issuer, err error) {
	now := time.Now()

	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}

		ob := r.vec.WithLabelValues(r.name, "Create", result)
		ob.Observe(time.Since(now).Seconds())
	}()

	rt, err = r.repo.Create(ctx, dbi, req)

	return rt, err
}
