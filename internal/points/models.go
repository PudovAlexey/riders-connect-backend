package points

import (
	"time"

	"github.com/google/uuid"
)

// Point categories. The backend stores these keys; human-readable labels live on
// the client. Note: "motorcyclists" is intentionally NOT a category here — riders
// are served by the existing /geo/users layer and toggled client-side.
const (
	CategoryTowing       = "towing"        // Мото эвакуатор
	CategoryTireService  = "tire_service"  // Шиномонтаж
	CategoryService      = "service"       // Сервис
	CategoryDealer       = "dealer"        // Мото салон
	CategoryGear         = "gear"          // Мото экипировка
	CategoryWash         = "wash"          // Мойки
	CategoryBar          = "bar"           // Бар для мотоциклистов
	CategoryScenic       = "scenic"        // Красивые места
	CategoryRacetrack    = "racetrack"     // Мототрек
	CategoryGymkhana     = "gymkhana"      // Мотоджимхана
	CategoryGasStation   = "gas_station"   // Заправка / АЗС
	CategoryParking      = "parking"       // Парковка для мото
	CategoryCamping      = "camping"       // Кемпинг / ночлег
	CategoryCafe         = "cafe"          // Кафе / место отдыха
	CategoryRidingSchool = "riding_school" // Мотошкола
	CategoryRental       = "rental"        // Прокат мото
	CategoryParts        = "parts"         // Запчасти
	CategoryTuning       = "tuning"        // Тюнинг-ателье
	CategoryPolice       = "police"        // Камеры и посты ДПС
	CategoryHazard       = "hazard"        // Опасные участки дороги
)

var validCategories = map[string]bool{
	CategoryTowing:       true,
	CategoryTireService:  true,
	CategoryService:      true,
	CategoryDealer:       true,
	CategoryGear:         true,
	CategoryWash:         true,
	CategoryBar:          true,
	CategoryScenic:       true,
	CategoryRacetrack:    true,
	CategoryGymkhana:     true,
	CategoryGasStation:   true,
	CategoryParking:      true,
	CategoryCamping:      true,
	CategoryCafe:         true,
	CategoryRidingSchool: true,
	CategoryRental:       true,
	CategoryParts:        true,
	CategoryTuning:       true,
	CategoryPolice:       true,
	CategoryHazard:       true,
}

type Point struct {
	ID          uuid.UUID `json:"id"`
	CreatedBy   uuid.UUID `json:"created_by"`
	Category    string    `json:"category"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	Address     string    `json:"address"`
	Phone       string    `json:"phone"`
	Website     string    `json:"website"`
	Hours       string    `json:"hours"`
	Photos      []string  `json:"photos"`
	// AvgRating and ReviewCount are aggregates over point_reviews, filled on reads
	// (List/Get). A point with no reviews reports 0/0.
	AvgRating   float64   `json:"avg_rating"`
	ReviewCount int       `json:"review_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
