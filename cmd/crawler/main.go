package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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

//var OpenDataParisEndpointURI = "https://opendata.paris.fr/api/records/1.0/search/?dataset=velib-disponibilite-en-temps-reel"
var OpenDataParisEndpointURI = "https://www.velib-metropole.fr/webapi/map/details?gpsTopLatitude=49.02511241985949&gpsTopLongitude=2.9344939906150107&gpsBotLatitude=48.77685694769403&gpsBotLongitude=1.772003145888448&zoomLevel=11"

/*
Stream should contains a json array containing Station objects
*/

type callback func(interface{}) bool

func importStationODP(r io.Reader, cb callback) (int, error) {
	var updated int

	// Read all
	body, err := ioutil.ReadAll(r)
	if err != nil {
		return updated, err
	}

	var records *[]common.ODPRecords

	records = &[]common.ODPRecords{}
	var ret common.ODPliveResponse
	err = json.Unmarshal(body, &ret)
	if err != nil {
		// Try for only records (json file)
		err = json.Unmarshal(body, records)
		if err != nil {
			return updated, err
		}
	} else {
		records = &ret.Records
	}

	log.Println("Records length ", len(*records))
	for _, rec := range *records {
		var m common.Station
		if rec.Fields.IsRenting == 0 ||
			rec.Fields.IsReturning == 0 ||
			rec.Fields.IsInstalled == 0 {
			continue
		}
		m.StationId = rec.Fields.StationId
		m.Name = rec.Fields.Name
		m.Position.Lat, err = rec.Fields.Lat.Float64()
		if err != nil {
			m.Position.Lat = 0.0
		}
		m.Position.Lng, err = rec.Fields.Lon.Float64()
		if err != nil {
			m.Position.Lng = 0.0
		}
		m.AvailableBikes = int64(rec.Fields.Numbikesavailable)
		m.AvailableBikeStands = int64(rec.Fields.Numdocksavailable)
		m.LastUpdate = int64(rec.Fields.LastReported)
		if cb(&m) {
			updated++
		}
	}

	return updated, nil
}

func writeStation(staChan chan *common.Station, contractname string, clnt tsdbClient.Client) {
	bp, _ := tsdbClient.NewBatchPoints(tsdbClient.BatchPointsConfig{
		Database:  contractname,
		Precision: "ms", //TODO
	})

	for station := range staChan {
		tags := map[string]string{
			"station_name": station.Name,
			"status":       station.Status,
			"station_id":   strconv.Itoa(station.StationId),
		}

		fields := map[string]interface{}{
			"bike_stands":           station.BikeStands,
			"available_bike_stands": station.AvailableBikeStands,
			"available_bikes":       station.AvailableBikes,
		}
		//t := time.Unix(station.LastUpdate, 0).Add(time.Duration(station.LastUpdate) * time.Millisecond)
		pt, err := tsdbClient.NewPoint("stations", tags, fields, time.Now())
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

func updateODPStations(url string, contract string, db *gorm.DB, clnt tsdbClient.Client) (int, error) {
	res, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	staChan := make(chan *common.Station)
	go writeStation(staChan, contract, clnt)

	updated, err := importStation(res.Body, func(s interface{}) bool {
		var _sta common.Station
		sta := s.(*common.Station)
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
	url := OpenDataParisEndpointURI
	updateODPStations(url, "paris", db, tsdbClient) //TODO remove me
	log.Print("First update done")
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		updated, err := updateODPStations(url, "paris", db, tsdbClient)
		if err != nil {
			log.Print(err)
			continue
		}
		log.Printf("%d updated stations", updated)
	}
}

func initStation(c *cli.Context, db *gorm.DB) {
	db.AutoMigrate(&common.Point{})
	db.AutoMigrate(&common.Station{})

	var count int
	db.Model(&common.Station{}).Count(&count)
	log.Println("Stations in databases: ", count)

	filepath := c.String("stations-json")
	if filepath != "" {
		file, err := os.Open(filepath)
		if err != nil {
			log.Fatal("Could not open ", filepath, err)
		}
		defer file.Close()
		count, err := importStation(file, func(s interface{}) bool { //TODO tests
			var _s common.Station
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

func importStation(r io.Reader, cb callback) (int, error) {
	var updated int

	// Read all
	body, err := ioutil.ReadAll(r)
	if err != nil {
		return updated, err
	}

	var records []common.VelibMetroppole

	err = json.Unmarshal(body, &records)
	if err != nil {
		return updated, err
	}

	log.Println("Records length ", len(records))
	for _, rec := range records {
		var m common.Station
		staId, err := strconv.Atoi(rec.Station.Code)
		if err != nil {
			continue
		}
		m.StationId = staId
		m.Name = rec.Station.Name
		m.AvailableBikes = int64(rec.Numbikesavailable + rec.NumbikesavailableOF + rec.NumEbikesavailable + rec.NumEbikesavailableOF)
		m.AvailableBikeStands = int64(rec.Numdocksavailable + rec.NumEdocksavailable)
		m.LastUpdate = int64(rec.Station.LastReported)

		m.Position.Lat, err = rec.Station.GPS.Lat.Float64()
		if err != nil {
			m.Position.Lat = 0.0
		}
		m.Position.Lng, err = rec.Station.GPS.Lon.Float64()
		if err != nil {
			m.Position.Lng = 0.0
		}
		if cb(&m) {
			updated++
		}
	}

	return updated, nil
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
	q := tsdbClient.NewQuery(fmt.Sprint("CREATE DATABASE ", "paris"), "", "")
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
	}
	log.SetOutput(os.Stdout)
	app.Run(os.Args)
}
