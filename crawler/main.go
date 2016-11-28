package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	tsdbClient "github.com/influxdata/influxdb/client/v2"

	"github.com/codegangsta/cli"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

var jcdEndpointURI = "https://api.jcdecaux.com/vls/v1/stations?contract=%s&apiKey=%s"

type Point struct {
	Lat                 float64 `json:"lat"`
	Lng                 float64 `json:"lng"`
}

type Station struct {
	gorm.Model
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

/*
Stream should contains a json array containing Station objects
*/

type callback func(interface{}) bool

func importStation(r io.Reader, cb callback) (int, error) {
	var updated int

	d := json.NewDecoder(r)
	_, err := d.Token()
	if err != nil {
		return updated, err
	}

	for d.More() {
		var m Station
		err = d.Decode(&m)
		if err != nil {
			return updated, err
		}
		if cb(&m) {
			updated++
		}
	}
	_, err = d.Token()
	if err != nil {
		return updated, err
	}

	return updated, nil
}

func writeStation(staChan chan *Station , contractname string, clnt tsdbClient.Client) {
	bp, _ := tsdbClient.NewBatchPoints(tsdbClient.BatchPointsConfig{
		Database: contractname,
		Precision: "ms", //TODO
	})

	for station := range staChan {
		tags := map[string]string{
			"station_name":    station.Name,
			"status": station.Status,
			"station_id": strconv.Itoa(station.StationId),
		}

		fields := map[string]interface{}{
			"bike_stands": station.BikeStands,
			"available_bike_stands": station.AvailableBikeStands,
			"available_bikes": station.AvailaibleBikes,
		}
		t := time.Unix(0, 0).Add(time.Duration(station.LastUpdate) * time.Millisecond)
		pt, err := tsdbClient.NewPoint("stations", tags, fields, t)
		if err != nil {
			log.Print("Error while creating new point: ", err)
			continue
		}
		bp.AddPoint(pt)
	}

	err := clnt.Write(bp)
	if err != nil {
		log.Print("Error while writing points: ", err)
	}
}

func updateJCDStations(url string, contract string, db *gorm.DB, clnt tsdbClient.Client) (int, error) {
	res, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	staChan := make(chan *Station)
	go writeStation(staChan, contract, clnt)

	updated, err := importStation(res.Body, func(s interface{}) bool {
		var _sta Station
		sta := s.(*Station)
		db.FirstOrInit(&_sta, sta)
		/*
			This check is useful to prevent external provider to correct past
			history, developer should get notified if that happens
		*/
		if _sta.InternalLastUpdate > sta.LastUpdate {
			log.Printf("Not updating station %d timestamp too early\n", sta.StationId)
			return false
		}
		staChan <- sta
		//Check for differency in field and log it before modification
		_sta.InternalLastUpdate = sta.LastUpdate
		db.Save(&_sta)
		return true
	})

	close(staChan)
	return updated, err
}

func watchdogStations(c *cli.Context, db *gorm.DB, tsdbClient tsdbClient.Client) {
	url := fmt.Sprintf(jcdEndpointURI, c.String("jcd-contract"), c.String("jcd-api-key"))
	updateJCDStations(url, c.String("jcd-contract"), db, tsdbClient) //TODO remove me
	log.Print("First update done")
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		updated, err := updateJCDStations(url, c.String("jcd-contract"), db, tsdbClient)
		if err != nil {
			log.Print(err)
			continue
		}
		log.Printf("%d updated stations", updated)
	}
	// Every minute after a successful GET start the following function
	// GET JSON with jcd-api-key and jcd-contract at
	// For each station
	// Update db if new station and show it
	// Continue to next entry if internal last update is after JCD last update
	// Put data in tsdbClient for everything can be updated
}

func initStation(c *cli.Context, db *gorm.DB) {
	db.AutoMigrate(&Point{})
	db.AutoMigrate(&Station{})

	var count int
	db.Model(&Station{}).Count(&count)
	log.Println("Stations in databases: ", count)

	filepath := c.String("stations-json")
	if filepath != "" {
		file, err := os.Open(filepath)
		if err != nil {
			log.Fatal("Could not open", filepath, err)
		}
		defer file.Close()
		count, err := importStation(file, func(s interface{}) bool { //TODO tests
			var _s Station
			var ret bool
			if db.NewRecord(s) {
				ret = true
			}
			db.FirstOrInit(&_s, s)
			return ret
		})
		if err != nil {
			log.Fatal("Error while importing", filepath, err)
		}
		log.Printf("Databases station updated with %v new record(s)\n", count)
	}
}

func run(c *cli.Context) {
	var err error
	var db *gorm.DB
	for i := uint(0); i < 5; i++ {
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
	q := tsdbClient.NewQuery(fmt.Sprint("CREATE DATABASE ", c.String("jcd-contract")), "", "")
	if response, err := clnt.Query(q); err == nil && response.Error() == nil {
		fmt.Println(response.Results)
	}
	if err != nil {
		log.Fatal(err)
	}
	go watchdogStations(c, db, clnt)

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
			Name:   "jcd-api-key",
			Usage:  "JCDecaux API key (see https://developer.jcdecaux.com/",
			EnvVar: "JCD_API_KEY",
		},
		cli.StringFlag{
			Name:   "jcd-contract",
			Usage:  "JCDecaux contract name see (see https://developer.jcdecaux.com/",
			EnvVar: "JCD_CONTRACT",
		},
	}
	log.SetOutput(os.Stdout)
	app.Run(os.Args)
}
