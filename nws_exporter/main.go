package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	station              string
	address              string
	help                 bool
	verbose              bool
	timeout, backofftime int
	failfast             bool
	localaddr            string

	humidity = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "humidity",
		Help:      "humidity gauge percentage",
	})
	temperature = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "temperature",
		Help:      "temperature in celsius",
	})
	dewpoint = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "dewpoint",
		Help:      "dewpoint in celsius",
	})
	winddirection = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "nws",
			Name:      "wind_direction",
			Help:      "wind direction in degrees",
		},
		[]string{"Direction"},
	)
	windspeed = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "wind_speed",
		Help:      "wind speed in kilometers per hour",
	})
	barometricpressure = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "barometric_pressure",
		Help:      "barometric pressure in pascals",
	})
	sealevelpressure = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "sealevel_pressure",
		Help:      "sealevel pressure in pascals",
	})
	visibility = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "nws",
		Name:      "visibility",
		Help:      "visibility in meters",
	})
	cloudcover = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "nws",
			Name:      "cloud_cover",
			Help:      "cloud cover amount and base height in meters",
		},
		[]string{"amount"},
	)
	sunAltitude = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sun",
		Name:      "altitude",
		Help:      "sun altitude in degrees above horizon (negative = below horizon)",
	})
	sunAzimuth = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sun",
		Name:      "azimuth",
		Help:      "sun azimuth in degrees from North (0=N, 90=E, 180=S, 270=W)",
	})
	sunIsDaylight = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sun",
		Name:      "is_daylight",
		Help:      "1 if sun is above horizon, 0 if below",
	})
	sunSunrise = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sun",
		Name:      "sunrise_time",
		Help:      "today's sunrise time as Unix timestamp",
	})
	sunSunset = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "sun",
		Name:      "sunset_time",
		Help:      "today's sunset time as Unix timestamp",
	})
)

func init() {
	flag.StringVar(&station, "station", "KPHL", "nws address")
	flag.StringVar(&localaddr, "localaddr", ":8080", "The address to listen on for HTTP requests")
	flag.StringVar(&address, "addr", "api.weather.gov", "nws address")
	flag.BoolVar(&help, "help", false, "help info")
	flag.BoolVar(&verbose, "verbose", false, "verbose logging")
	flag.IntVar(&timeout, "timeout", 10, "timeout in seconds")
	flag.IntVar(&backofftime, "backofftime", 100, "backofftime in seconds")
	flag.BoolVar(&failfast, "failfast", false, "Exit quickly on errors")
	flag.Parse()
	prometheus.MustRegister(humidity)
	prometheus.MustRegister(temperature)
	prometheus.MustRegister(dewpoint)
	prometheus.MustRegister(winddirection)
	prometheus.MustRegister(windspeed)
	prometheus.MustRegister(barometricpressure)
	prometheus.MustRegister(sealevelpressure)
	prometheus.MustRegister(visibility)
	prometheus.MustRegister(cloudcover)
	prometheus.MustRegister(sunAltitude)
	prometheus.MustRegister(sunAzimuth)
	prometheus.MustRegister(sunIsDaylight)
	prometheus.MustRegister(sunSunrise)
	prometheus.MustRegister(sunSunset)
}

func main() {
	if help {
		flag.Usage()
		os.Exit(1)
	}

	log.Printf("Starting up, retrieving from %s at station %s", address, station)
	log.Printf("Serving on http://%s/metrics...", localaddr)
	// start scrape loop
	go func() {
		for {
			// Always try primary station first
			primaryResponse, primaryErr := RetrieveCurrentObservation(station, address, timeout)
			
			// Try fallback stations if needed: PHHN (Hana), PHLI (Lihue)
			fallbackStations := []string{"PHHN", "PHLI"}
			var fallbackResponse ObservationResponse
			var fallbackErr error
			fallbackUsed := false
			
			// Check if we need fallback data (primary has null temperature)
			if primaryErr != nil || primaryResponse.Properties.Temperature.Value == 0 {
				for _, tryStation := range fallbackStations {
					fallbackResponse, fallbackErr = RetrieveCurrentObservation(tryStation, address, timeout)
					if fallbackErr == nil && fallbackResponse.Properties.Temperature.Value != 0 {
						log.Printf("Using fallback station %s for missing data from %s", tryStation, station)
						fallbackUsed = true
						break
					}
				}
			}
			
			if primaryErr != nil && (!fallbackUsed || fallbackErr != nil) {
				if failfast {
					log.Fatalf("error: %v", primaryErr)
				}

				log.Printf("Problem retrieving from all stations: %v", primaryErr)
				backoffseconds := (time.Duration(backofftime) * time.Second)
				log.Printf("Waiting %v seconds, next scrape at %s", backofftime, time.Now().Add(backoffseconds))
				time.Sleep(time.Duration(backofftime) * time.Second)
				continue
			}
			
			// Helper function to get value from primary or fallback
			getValue := func(primaryVal, fallbackVal float64) float64 {
				if primaryErr == nil && primaryVal != 0 {
					return primaryVal
				}
				if fallbackUsed && fallbackVal != 0 {
					return fallbackVal
				}
				return 0
			}
			
			// Set metrics, preferring primary station data
			if val := getValue(primaryResponse.Properties.RelativeHumidity.Value, fallbackResponse.Properties.RelativeHumidity.Value); val != 0 {
				humidity.Set(val)
			}
			if val := getValue(primaryResponse.Properties.Temperature.Value, fallbackResponse.Properties.Temperature.Value); val != 0 {
				temperature.Set(val)
			}
			if val := getValue(primaryResponse.Properties.Dewpoint.Value, fallbackResponse.Properties.Dewpoint.Value); val != 0 {
				dewpoint.Set(val)
			}
			if val := getValue(primaryResponse.Properties.WindDirection.Value, fallbackResponse.Properties.WindDirection.Value); val != 0 {
				winddirection.WithLabelValues(CardinalDirection(val)).Set(val)
			}
			if val := getValue(primaryResponse.Properties.WindSpeed.Value, fallbackResponse.Properties.WindSpeed.Value); val != 0 {
				windspeed.Set(val)
			}
			if val := getValue(primaryResponse.Properties.BarometricPressure.Value, fallbackResponse.Properties.BarometricPressure.Value); val != 0 {
				barometricpressure.Set(val)
			}
			if val := getValue(primaryResponse.Properties.SeaLevelPressure.Value, fallbackResponse.Properties.SeaLevelPressure.Value); val != 0 {
				sealevelpressure.Set(val)
			}
			if val := getValue(primaryResponse.Properties.Visibility.Value, fallbackResponse.Properties.Visibility.Value); val != 0 {
				visibility.Set(val)
			}
			
			// Cloud cover - always prefer primary station (PHOG)
			if primaryErr == nil && len(primaryResponse.Properties.CloudLayers) > 0 {
				for _, layer := range primaryResponse.Properties.CloudLayers {
					baseHeight := 0.0
					if layer.Base.Value != 0 {
						baseHeight = float64(layer.Base.Value)
					}
					cloudcover.WithLabelValues(layer.Amount).Set(baseHeight)
				}
			} else if fallbackUsed && len(fallbackResponse.Properties.CloudLayers) > 0 {
				for _, layer := range fallbackResponse.Properties.CloudLayers {
					baseHeight := 0.0
					if layer.Base.Value != 0 {
						baseHeight = float64(layer.Base.Value)
					}
				cloudcover.WithLabelValues(layer.Amount).Set(baseHeight)
				}
			}
			
			// Calculate and set sun position
			sunPos := CalculateSunPosition(time.Now())
			sunAltitude.Set(sunPos.Altitude)
			sunAzimuth.Set(sunPos.Azimuth)
			if sunPos.IsDaylight {
				sunIsDaylight.Set(1)
			} else {
				sunIsDaylight.Set(0)
			}
			if !sunPos.Sunrise.IsZero() {
				sunSunrise.Set(float64(sunPos.Sunrise.Unix()))
			}
			if !sunPos.Sunset.IsZero() {
				sunSunset.Set(float64(sunPos.Sunset.Unix()))
			}
			
			if verbose {
				log.Printf("Sun: alt=%.1f°, az=%.1f°, daylight=%v", sunPos.Altitude, sunPos.Azimuth, sunPos.IsDaylight)
				log.Printf("Sunrise: %s, Sunset: %s", sunPos.Sunrise.Format("2006-01-02 15:04 MST"), sunPos.Sunset.Format("2006-01-02 15:04 MST"))
				log.Printf("Waiting %v seconds, next scrape at %s", backofftime, time.Now().Add(
					time.Duration(backofftime)*time.Second).String())
			}
			time.Sleep(time.Duration(backofftime) * time.Second)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(localaddr, nil))
}
