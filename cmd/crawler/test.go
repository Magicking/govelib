package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

var OpenDataParisEndpointURI = "https://opendata.paris.fr/api/records/1.0/search/?dataset=velib-disponibilite-en-temps-reel"

func main() {
	// Request
	res, err := http.Get(OpenDataParisEndpointURI)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()

	// Read all
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	var ret struct 
		Nhits   int `json:"nhits"`
		Records []struct {
			Datasetid string `json:"datasetid"`
			Recordid  string `json:"recordid"`
			Fields    struct {
				Capacity          int           `json:capacity`
				Name              string        `json:name`
				Numbikesavailable int           `json:numbikesavailable`
				LastReported      int           `json:last_reported`
				Lon               json.Number   `json:lon`
				StationId         int           `json:station_id`
				Lat               json.Number   `json:lat`
				Xy                []json.Number `json:xy`
				IsInstalled       int           `json:is_installed`
				IsRenting         int           `json:is_renting`
				Numdocksavailable int           `json:numdocksavailable`
				IsReturning       int           `json:is_returning`
			} `json:"fields"`
		} `json:"records"`
	}
	json.Unmarshal(body, &ret)
	log.Println(ret)
	//log.Println(stations)
	//log.Println(stations["fields"])
	//log.Println(reflect.TypeOf(msg["records"]))
}
