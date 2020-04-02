package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"gopkg.in/yaml.v2"
)

type StatesData struct {
	Data []map[string]uint64
}

type NowData struct {
	CurrentTotals map[string]uint64 `json:"current_totals"`
}

// Config represents service configuration
// Field names should be public in order to correctly populate fields
type Config struct {
	configFile string
	Listen     string `yaml:"listen"`
	DefaultTTL string `yaml:"defaultTTL"`
	Metrics    struct {
		Total  string `yaml:"total"`
		States []struct {
			Name string
			Data string
			TTL  string
		}
	} `yaml:"metrics"`
}

// Measurement represents single measurement
type Measurement struct {
	value    string
	executed time.Time
}

func (c *Config) getConfig() *Config {
	c.Listen = ":7070" //default listen port
	yamlFile, err := ioutil.ReadFile(c.configFile)
	if err != nil {
		log.Fatalf("Reading config: %v", err)
	}
	err = yaml.Unmarshal(yamlFile, &c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}
	return c
}

func main() {

	var c Config

	flag.StringVar(&c.configFile, "config", "config.yml", "Path to config.yml file")
	flag.Parse()

	c.getConfig()

	log.Println("Started COVID-DE exporter")

	metrics := map[string]Measurement{}

	for _, mtr := range c.Metrics.States {
		metrics[mtr.Name] = Measurement{
			value:    "0",
			executed: time.Unix(0, 0),
		}
	}

	metrics["total"] = Measurement{
		value:    "0",
		executed: time.Unix(0, 0),
	}

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {

		// Get Totals gauge

		duration, _ := time.ParseDuration(c.DefaultTTL + "s")
		if time.Now().Unix() > metrics["total"].executed.Add(duration).Unix() {
			resp, err := http.Get(c.Metrics.Total)
			if err != nil {
				log.Println("Error getting total value: ", err.Error())
			} else {

				_, _ = fmt.Fprint(w, "# TYPE covid_de_total gauge\n")

				res := NowData{}
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Println("Error getting value: ", err.Error())
				}

				_ = json.Unmarshal(body, &res)

				for jj, kk := range res.CurrentTotals {
					_, _ = fmt.Fprintf(w, "covid_de_total{type=\"%s\"} %d\n", jj, kk)
				}
				fmt.Fprint(w, "\n")

				metrics["total"] = Measurement{
					value:    string(res.CurrentTotals["cases"]),
					executed: time.Now(),
				}

			}
		}

		_, _ = fmt.Fprint(w, "# TYPE covid_de_states gauge\n")

		for _, metricConfig := range c.Metrics.States {

			if metricConfig.TTL == "" {
				metricConfig.TTL = c.DefaultTTL
			}
			duration, _ := time.ParseDuration(metricConfig.TTL + "s")
			if time.Now().Unix() > metrics[metricConfig.Name].executed.Add(duration).Unix() {

				log.Println("Recalculating " + metricConfig.Name)

				res := StatesData{}
				value := "0"
				resp, err := http.Get(metricConfig.Data)
				if err != nil {
					log.Println("Error getting value: ", err.Error())
					value = metrics[metricConfig.Name].value
				} else {

					body, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						log.Println("Error getting value: ", err.Error())
						value = metrics[metricConfig.Name].value
					}

					_ = json.Unmarshal(body, &res)

					for _, jj := range res.Data[len(res.Data)-1] {
						value = fmt.Sprintf("%d", jj)
					}
				}

				metrics[metricConfig.Name] = Measurement{
					value:    value,
					executed: time.Now(),
				}

			}

			_, _ = fmt.Fprintf(w, "covid_de_states{state=\"%s\"} %s\n", metricConfig.Name, string(metrics[metricConfig.Name].value))
		}
	})
	log.Println("Listening for prometheus on " + c.Listen + "/metrics")
	log.Fatal(http.ListenAndServe(c.Listen, nil))
}
