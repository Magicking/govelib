package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tsdbClient "github.com/influxdata/influxdb/client/v2"

	"github.com/Magicking/govelib/common"
	"github.com/codegangsta/cli"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

type GeoCache struct {
	db         *gorm.DB
	Attractors interface{}
	Repulsors  interface{}
	Activities interface{}
}

func NewGeoCache(db *gorm.DB) *GeoCache {
	return &GeoCache{db: db}
}

func (gc *GeoCache) GetCoordinates(stationId int) (lat, lng float64, err error) {
	var station common.Station
	// get all & cache results
	if gc.db.Where(common.Station{StationId: stationId}).First(&station).RecordNotFound() {
		err = fmt.Errorf("Station %v not in database", stationId)
		return
	}

	lat = station.Position.Lat
	lng = station.Position.Lng
	return
}

func updateHeatMap(contract, delta, timeLimit string, geoCache *GeoCache, tsClient *tsdbClient.Client) error {
	queryString := fmt.Sprintf(`SELECT "available_bikes", "available_bike_stands" FROM "stations" WHERE time > %s - %s GROUP BY station_id`, timeLimit, delta)
	q := tsdbClient.NewQuery(queryString, contract, "1m")
	ret, err := (*tsClient).Query(q)
	if err != nil {
		log.Print(ret.Err)
		return err
	}

	var attractors [][]interface{}
	var repulsors [][]interface{}
	var activities [][]interface{}
	var availableBikesDeltaLow int64
	var availableBikesDeltaHigh int64
	var availableBikeStandsDeltaLow int64
	var availableBikeStandsDeltaHigh int64
	var operationCountMax int64
	for _, row := range ret.Results[0].Series {
		availableBikesBefore, err := row.Values[0][1].(json.Number).Int64()
		if err != nil {
			return err
		}
		availableBikesNow, err := row.Values[len(row.Values)-1][1].(json.Number).Int64()
		if err != nil {
			return err
		}
		availableBikesDelta := availableBikesNow - availableBikesBefore

		availableBikeStandsBefore, err := row.Values[0][2].(json.Number).Int64()
		if err != nil {
			return err
		}
		availableBikeStandsNow, err := row.Values[len(row.Values)-1][2].(json.Number).Int64()
		if err != nil {
			return err
		}
		availableBikeStandsDelta := availableBikeStandsNow - availableBikeStandsBefore

		lastCount1 := availableBikesBefore
		lastCount2 := availableBikeStandsBefore
		var operationCount int64
		for _, val := range row.Values {
			op1, err := val[1].(json.Number).Int64()
			if err != nil {
				return err
			}
			op2, err := val[2].(json.Number).Int64()
			if err != nil {
				return err
			}
			if lastCount1 > op1 {
				operationCount += lastCount1 - op1
			} else {
				operationCount += op1 - lastCount1
			}
			if lastCount2 > op2 {
				operationCount += lastCount2 - op2
			} else {
				operationCount += op2 - lastCount2
			}
			lastCount1 = op1
			lastCount2 = op2
		}
		station_id, err := strconv.Atoi(row.Tags["station_id"])
		if err != nil {
			return err
		}
		lat, lng, err := geoCache.GetCoordinates(station_id)
		if err != nil {
			log.Print(err)
			continue
		}
		if availableBikesDelta < availableBikesDeltaLow {
			availableBikesDeltaLow = availableBikesDelta
		}
		if availableBikesDelta > availableBikesDeltaHigh {
			availableBikesDeltaHigh = availableBikesDelta
		}
		if availableBikeStandsDelta < availableBikeStandsDeltaLow {
			availableBikeStandsDeltaLow = availableBikeStandsDelta
		}
		if availableBikeStandsDelta > availableBikeStandsDeltaHigh {
			availableBikeStandsDeltaHigh = availableBikeStandsDelta
		}
		if operationCount > operationCountMax {
			operationCountMax = operationCount
		}
		attractors = append(attractors, []interface{}{lat, lng, availableBikesDelta})
		repulsors = append(repulsors, []interface{}{lat, lng, availableBikeStandsDelta})
		activities = append(activities, []interface{}{lat, lng, operationCount})
	}
	var attractorDelta float64
	var repulsorDelta float64
	if availableBikesDeltaLow < 0 {
		attractorDelta = -float64(availableBikesDeltaLow)
	}
	if availableBikeStandsDeltaLow < 0 {
		repulsorDelta = -float64(availableBikeStandsDeltaLow)
	}
	// normalize
	for _, element := range attractors {
		value := (float64(element[2].(int64)) + attractorDelta) / float64(availableBikesDeltaHigh)
		element[2] = &value
	}
	for _, element := range repulsors {
		value := (float64(element[2].(int64)) + repulsorDelta) / float64(availableBikeStandsDeltaHigh)
		element[2] = &value
	}
	for _, element := range activities {
		value := float64(element[2].(int64)) / float64(operationCountMax)
		element[2] = &value
	}
	geoCache.Attractors = attractors
	geoCache.Repulsors = repulsors
	geoCache.Activities = activities
	return nil
}

func watchdogStations(c *cli.Context, geoCache *GeoCache, tsdbClient *tsdbClient.Client) {
	ticker := time.NewTicker(10 * time.Second)
	delta := "5m"
	timeLimit := "now()"
	for range ticker.C {
		err := updateHeatMap(c.String("city"), delta, timeLimit, geoCache, tsdbClient)
		if err != nil {
			log.Print(err)
			continue
		}
		log.Printf("Updated stations")
	}
}

func initStation(c *cli.Context, db *gorm.DB) {
	db.AutoMigrate(&common.Point{})
	db.AutoMigrate(&common.Station{})

	var count int
	db.Model(&common.Station{}).Count(&count)
	log.Println("Stations in databases: ", count)
}

func run(c *cli.Context) {
	var err error
	var db *gorm.DB
	for i := uint(0); i < 10; i++ {
		db, err = gorm.Open("postgres", c.String("postgres-dsn"))
		if err == nil {
			break
		}
		log.Println("While connecting to station database", err)
		time.Sleep((1 << i) * time.Second)
	}
	if err != nil {
		log.Fatalln("While connecting to station database", err)
	}
	defer db.Close()
	go initStation(c, db)
	clnt, err := tsdbClient.NewHTTPClient(tsdbClient.HTTPConfig{
		Addr: c.String("influxdb-uri"),
	})
	if err != nil {
		log.Fatalln("InfluxDB connection:", err)
	}
	defer clnt.Close()
	geoCache := NewGeoCache(db)
	go watchdogStations(c, geoCache, &clnt)
	http.HandleFunc("/map/activities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(geoCache.Activities); err != nil {
			log.Print(err)
		}
	})
	http.HandleFunc("/map/attractors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(geoCache.Attractors); err != nil {
			log.Print(err)
		}
	})
	http.HandleFunc("/map/repulsors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if err := json.NewEncoder(w).Encode(geoCache.Repulsors); err != nil {
			log.Print(err)
		}
	})

	go func() {
		log.Println("Listening on 0.0.0.0:8080")
		log.Fatal(http.ListenAndServe("0.0.0.0:8080", nil))
	}()

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	log.Println(<-sigChan)
}

func main() {
	app := cli.NewApp()
	app.Name = "govelib"
	app.Usage = "Govelib"
	app.Action = run
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "postgres-dsn",
			Usage:  "postgresql dsn (e.g.: host=myhost user=gorm dbname=govelib sslmode=disable password=mypassword",
			EnvVar: "POSTGRES_DSN",
		},
		cli.StringFlag{
			Name:   "influxdb-uri",
			Usage:  "influxdb uri (e.g.: http://influxuser:influxpassword@hostname:port/)",
			EnvVar: "INFLUXDB_URI",
		},
		cli.StringFlag{
			Name:   "stations-json",
			Usage:  "json stations file path (e.g.: stations.json)",
			EnvVar: "STATIONS_JSON",
		},
		cli.StringFlag{
			Name:   "city",
			Usage:  "City name",
			EnvVar: "CITY",
		},
	}
	log.SetOutput(os.Stdout)
	app.Run(os.Args)
}
