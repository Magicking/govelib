package main

import (
	"github.com/jinzhu/gorm"
)

type Point struct {
	Lat            float64 `json:"lat"`
	Lng           float64 `json:"lng"`
}

type Station struct {
	gorm.Model          `json:"-"`
	StationId           int   `json:"number" gorm:"unique_index"` // unique only within contract
	Name                string  `json:"name"`
	Address             string  `json:"address"`
	Position            Point   `json:"position" gorm:"embedded";embedded_prefix:position_`
	Status              string  `json:"status" gorm:"-"` // indicates whether this station is CLOSED or OPEN
	BikeStands          int64   `json:"bike_stands" gorm:"-"` // the number of operational bike stands at this station
	AvailableBikeStands int64   `json:"available_bike_stands" gorm:"-"` // the number of available bike stands at this station
	AvailaibleBikes     int64   `json:"available_bikes" gorm:"-"` // the number of available and operational bikes at this station
	LastUpdate          int64   `json:"last_update" gorm:"-"` // timestamp indicating the last update time in milliseconds since Epoch
	InternalLastUpdate  int64   `json:"-"`
}
