package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

var (
	listenAddress = flag.String("telemetry.address", ":9552", "Address on which to expose metrics.")
	metricsPath   = flag.String("telemetry.endpoint", "/metrics", "Path under which to expose metrics.")
	nightscoutUrl = flag.String("nightscout_endpoint", "https://foo.azurewebsites.net/pebble?count=2&units=mgdl", "Nightscout url to jsondata, only mmol is supported")
)

// Exporter collects nightscout stats from machine of a specified user and exports them using
// the prometheus metrics package.
type Exporter struct {
	mutex            sync.RWMutex
	statusNightscout *prometheus.GaugeVec
}

type NightscoutPebble struct {
	Status []struct {
		Now int64 `json:"now"`
	} `json:"status"`
	Bgs []struct {
		Sgv       string `json:"sgv"`
		Trend     int    `json:"trend"`
		Direction string `json:"direction"`
		Datetime  int64  `json:"datetime"`
		Bgdelta   string `json:"bgdelta"`
	} `json:"bgs"`
	Cals []interface{} `json:"cals"`
}

func getJson(url string) NightscoutPebble {
	//fmt.Println("fetching body from url", url)
	r, err := http.Get(url)
	if err != nil {
		fmt.Println("got error1", err.Error())
		return NightscoutPebble{}
	}
	defer r.Body.Close()

	bar := NightscoutPebble{}
	err2 := json.NewDecoder(r.Body).Decode(&bar)
	if err2 != nil {
		fmt.Println("error:", err2.Error())
		return NightscoutPebble{}
	}

	return bar

}

// NewNightscoutCheckerExporter returns an initialized Exporter.
func NewNightscoutCheckerExporter() *Exporter {

	return &Exporter{

		statusNightscout: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "nightscout",
				Name:      "nightscout_pebble",
				Help:      "checks current blood sugar from url",
			}, []string{"glucosetype", "url"}),
	}

}

// Describe describes all the metrics ever exported by the nightscout exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.statusNightscout.Describe(ch)
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) error {
	e.statusNightscout.Reset()

	data := getJson(*nightscoutUrl)

	fmt.Println("trying to convert to float:", data.Bgs[0].Sgv)
	glucose, err := strconv.ParseFloat(data.Bgs[0].Sgv, 64)
	if err != nil {
		e.statusNightscout.With(prometheus.Labels{"glucosetype": "mgdl", "url": *nightscoutUrl}).Set(float64(glucose))
	}

	return nil
}

// Collect fetches the stats of a user and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock() // To protect metrics from concurrent collects.
	defer e.mutex.Unlock()
	if err := e.scrape(ch); err != nil {
		log.Printf("Error scraping nightscout url: %s", err)
	}

	e.statusNightscout.Collect(ch)

	return
}

func main() {

	flag.Parse()

	exporter := NewNightscoutCheckerExporter()
	prometheus.MustRegister(exporter)
	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
                <head><title>Nightscout exporter</title></head>
                <body>
                   <h1>nightscout exporter</h1>
                   <p><a href='` + *metricsPath + `'>Metrics</a></p>
                   </body>
                </html>
              `))
	})
	log.Infof("Starting Server: %s", *listenAddress)
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
