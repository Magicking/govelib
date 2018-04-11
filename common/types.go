package common

import "encoding/json"

type VelibMetroppole struct {
	Station struct {
		GPS struct {
			Lon json.Number `json:"longitude"`
			Lat json.Number `json:"latitude"`
		} `json:"gps"`
		State        string `json:"state"`
		Name         string `json:"name"`
		Code         string `json:"code"`
		Type         string `json:"type"`
		LastReported int    `json:"dueDate"`
	} `json:"station"`
	Numbikesavailable    int `json:"nbBike"`
	NumEbikesavailable   int `json:"nbEbike"`
	NumbikesavailableOF  int `json:"nbBikeOverflow"`
	NumEbikesavailableOF int `json:"nbEBikeOverflow"`
	Numdocksavailable    int `json:"nbFreeDock"`
	NumEdocksavailable   int `json:"nbFreeEDock"`
}

type ODPRecords struct {
	Datasetid string `json:"datasetid"`
	Recordid  string `json:"recordid"`
	Fields    struct {
		Capacity          int         `json:"capacity"`
		Name              string      `json:"name"`
		Numbikesavailable int         `json:"numbikesavailable"`
		LastReported      int         `json:"last_reported"`
		Lon               json.Number `json:"lon,omitempty"`
		StationId         int         `json:"station_id"`
		Lat               json.Number `json:"lat,omitempty"`
		IsInstalled       int         `json:"is_installed"`
		IsRenting         int         `json:"is_renting"`
		Numdocksavailable int         `json:"numdocksavailable"`
		IsReturning       int         `json:"is_returning"`
	} `json:"fields"`
}

type ODPliveResponse struct {
	Nhits   int          `json:"nhits"`
	Records []ODPRecords `json:"records"`
}

type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Station struct {
	StationId           int    `json:"number" gorm:"index"` // unique only within contract
	Name                string `json:"name"`
	Address             string `json:"address"`
	Position            Point  `json:"position" gorm:"embedded";embedded_prefix:position_`
	Status              string `json:"status" gorm:"-"`                // indicates whether this station is CLOSED or OPEN
	BikeStands          int64  `json:"bike_stands" gorm:"-"`           // the number of operational bike stands at this station
	AvailableBikeStands int64  `json:"available_bike_stands" gorm:"-"` // the number of available bike stands at this station
	AvailableBikes      int64  `json:"available_bikes" gorm:"-"`       // the number of available and operational bikes at this station
	LastUpdate          int64  `json:"last_update" gorm:"-"`           // timestamp indicating the last update time in milliseconds since Epoch
	InternalLastUpdate  int64  `json:"-"`
}
