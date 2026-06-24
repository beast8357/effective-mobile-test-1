package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"effective-mobile/internal/model"
)

var ErrNotFound = errors.New("subscription not found")

type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

const selectColumns = "id, service_name, price, user_id, start_date, end_date"

func (r *SubscriptionRepository) Create(ctx context.Context, s *model.Subscription) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO subscriptions (id, service_name, price, user_id, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		s.ID, s.ServiceName, s.Price, s.UserID, s.StartDate.Time(), endDateValue(s.EndDate),
	)
	if err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}
	return nil
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Subscription, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM subscriptions WHERE id = $1`, id)
	s, err := scanSubscription(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("select subscription: %w", err)
	}
	return s, nil
}

func (r *SubscriptionRepository) Update(ctx context.Context, s *model.Subscription) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE subscriptions
		   SET service_name = $2,
		       price        = $3,
		       user_id      = $4,
		       start_date   = $5,
		       end_date     = $6,
		       updated_at   = NOW()
		 WHERE id = $1`,
		s.ID, s.ServiceName, s.Price, s.UserID, s.StartDate.Time(), endDateValue(s.EndDate),
	)
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SubscriptionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type ListFilter struct {
	UserID      *uuid.UUID
	ServiceName *string
	Limit       int
	Offset      int
}

func (r *SubscriptionRepository) List(ctx context.Context, f ListFilter) ([]model.Subscription, error) {
	var b strings.Builder
	b.WriteString(`SELECT ` + selectColumns + ` FROM subscriptions WHERE 1=1`)
	args := []any{}
	if f.UserID != nil {
		args = append(args, *f.UserID)
		fmt.Fprintf(&b, ` AND user_id = $%d`, len(args))
	}
	if f.ServiceName != nil {
		args = append(args, "%"+*f.ServiceName+"%")
		fmt.Fprintf(&b, ` AND service_name ILIKE $%d`, len(args))
	}
	b.WriteString(` ORDER BY start_date DESC, id`)
	if f.Limit > 0 {
		args = append(args, f.Limit)
		fmt.Fprintf(&b, ` LIMIT $%d`, len(args))
	}
	if f.Offset > 0 {
		args = append(args, f.Offset)
		fmt.Fprintf(&b, ` OFFSET $%d`, len(args))
	}
	rows, err := r.pool.Query(ctx, b.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	subs := make([]model.Subscription, 0)
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *s)
	}
	return subs, rows.Err()
}

type TotalFilter struct {
	From        time.Time
	To          time.Time
	UserID      *uuid.UUID
	ServiceName *string
}

// Total returns the aggregated subscription cost over [from, to].
// For each subscription overlapping the period we count the number of
// active months inside the window (inclusive) and multiply by the price.
func (r *SubscriptionRepository) Total(ctx context.Context, f TotalFilter) (int, error) {
	var b strings.Builder
	args := []any{f.From, f.To}
	b.WriteString(`
		WITH effective AS (
			SELECT
				price,
				GREATEST(start_date, $1::date) AS eff_start,
				LEAST(COALESCE(end_date, $2::date), $2::date) AS eff_end
			FROM subscriptions
			WHERE start_date <= $2::date
			  AND (end_date IS NULL OR end_date >= $1::date)`)
	if f.UserID != nil {
		args = append(args, *f.UserID)
		fmt.Fprintf(&b, ` AND user_id = $%d`, len(args))
	}
	if f.ServiceName != nil {
		args = append(args, "%"+*f.ServiceName+"%")
		fmt.Fprintf(&b, ` AND service_name ILIKE $%d`, len(args))
	}
	b.WriteString(`
		)
		SELECT COALESCE(SUM(price *
			((EXTRACT(YEAR FROM eff_end)::int - EXTRACT(YEAR FROM eff_start)::int) * 12
			+ (EXTRACT(MONTH FROM eff_end)::int - EXTRACT(MONTH FROM eff_start)::int) + 1)
		), 0)::bigint
		FROM effective
		WHERE eff_end >= eff_start`)

	var total int64
	if err := r.pool.QueryRow(ctx, b.String(), args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("total subscriptions: %w", err)
	}
	return int(total), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row rowScanner) (*model.Subscription, error) {
	var (
		s         model.Subscription
		startDate time.Time
		endDate   *time.Time
	)
	if err := row.Scan(&s.ID, &s.ServiceName, &s.Price, &s.UserID, &startDate, &endDate); err != nil {
		return nil, err
	}
	s.StartDate = model.MonthYear(startDate)
	if endDate != nil {
		my := model.MonthYear(*endDate)
		s.EndDate = &my
	}
	return &s, nil
}

func endDateValue(my *model.MonthYear) any {
	if my == nil {
		return nil
	}
	return my.Time()
}
