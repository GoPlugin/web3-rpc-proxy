package schema

import "github.com/jackc/pgx/pgtype"

type Tenant struct {
	Name        string        `gorm:"varchar(255); notNull;" json:"name"`
	Token       string        `gorm:"varchar(255); notNull;" json:"token"`
	Rate        float64       `gorm:"type:real; notNull;" json:"rate"`
	Capacity    float64       `gorm:"notNull;" json:"capacity"`
	Preferences *pgtype.JSONB `gorm:"type:jsonb; notNull; default:'{}'::jsonb;" json:"preferences"`

	Base
}
