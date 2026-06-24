package model

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const MonthYearLayout = "01-2006"

// MonthYear represents a date with month/year precision, serialized as "MM-YYYY".
// Internally it is normalised to the first day of the corresponding month (UTC).
type MonthYear time.Time

func NewMonthYear(year int, month time.Month) MonthYear {
	return MonthYear(time.Date(year, month, 1, 0, 0, 0, 0, time.UTC))
}

func ParseMonthYear(s string) (MonthYear, error) {
	t, err := time.Parse(MonthYearLayout, s)
	if err != nil {
		return MonthYear{}, fmt.Errorf("invalid date %q, expected MM-YYYY", s)
	}
	return MonthYear(t), nil
}

func (m MonthYear) Time() time.Time     { return time.Time(m) }
func (m MonthYear) String() string      { return time.Time(m).Format(MonthYearLayout) }
func (m MonthYear) IsZero() bool        { return time.Time(m).IsZero() }
func (m MonthYear) Before(o MonthYear) bool { return time.Time(m).Before(time.Time(o)) }

func (m MonthYear) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

func (m *MonthYear) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	v, err := ParseMonthYear(s)
	if err != nil {
		return err
	}
	*m = v
	return nil
}

type Subscription struct {
	ID          uuid.UUID  `json:"id"`
	ServiceName string     `json:"service_name"`
	Price       int        `json:"price"`
	UserID      uuid.UUID  `json:"user_id"`
	StartDate   MonthYear  `json:"start_date"`
	EndDate     *MonthYear `json:"end_date,omitempty"`
}
