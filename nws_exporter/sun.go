package main

import (
	"math"
	"time"
)

// Coordinates for Maui (PHOG - Kahului Airport)
const (
	latitude  = 20.8986  // degrees North
	longitude = -156.4306 // degrees West
)

// SunPosition calculates the sun's altitude and azimuth for the given time
type SunPosition struct {
	Altitude float64 // degrees above horizon (negative = below)
	Azimuth  float64 // degrees from North (0=N, 90=E, 180=S, 270=W)
	IsDaylight bool
	Sunrise  time.Time
	Sunset   time.Time
}

// CalculateSunPosition computes the sun position for the current time
func CalculateSunPosition(t time.Time) SunPosition {
	// Calculate sunrise/sunset for Hawaii local date first
	// This ensures we always get today's times in local timezone
	hst := time.FixedZone("HST", -10*3600) // Hawaii Standard Time
	localTime := t.In(hst)
	sunrise, sunset := calculateSunriseSunset(localTime, latitude, longitude)
	
	// Convert to UTC for sun position calculation
	t = t.UTC()
	
	// Calculate Julian day
	jd := toJulianDay(t)
	
	// Calculate sun position
	alt, az := sunPosition(jd, latitude, longitude)
	
	isDaylight := alt > -0.833 // Account for atmospheric refraction
	
	return SunPosition{
		Altitude: alt,
		Azimuth: az,
		IsDaylight: isDaylight,
		Sunrise: sunrise,
		Sunset: sunset,
	}
}

// toJulianDay converts a time to Julian Day
func toJulianDay(t time.Time) float64 {
	year := t.Year()
	month := int(t.Month())
	day := t.Day()
	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()
	
	if month <= 2 {
		year--
		month += 12
	}
	
	a := year / 100
	b := 2 - a + a/4
	
	jd := float64(int(365.25*float64(year+4716))) +
		float64(int(30.6001*float64(month+1))) +
		float64(day) + float64(b) - 1524.5
	
	dayFraction := (float64(hour) + float64(minute)/60.0 + float64(second)/3600.0) / 24.0
	
	return jd + dayFraction
}

// sunPosition calculates altitude and azimuth
func sunPosition(jd, lat, lon float64) (altitude, azimuth float64) {
	// Calculate number of days since J2000.0
	n := jd - 2451545.0
	
	// Mean longitude of the Sun
	L := math.Mod(280.460+0.9856474*n, 360.0)
	
	// Mean anomaly of the Sun
	g := math.Mod(357.528+0.9856003*n, 360.0)
	gRad := g * math.Pi / 180.0
	
	// Ecliptic longitude
	lambda := L + 1.915*math.Sin(gRad) + 0.020*math.Sin(2*gRad)
	lambdaRad := lambda * math.Pi / 180.0
	
	// Obliquity of ecliptic
	epsilon := 23.439 - 0.0000004*n
	epsilonRad := epsilon * math.Pi / 180.0
	
	// Right ascension
	alpha := math.Atan2(math.Cos(epsilonRad)*math.Sin(lambdaRad), math.Cos(lambdaRad))
	
	// Declination
	delta := math.Asin(math.Sin(epsilonRad) * math.Sin(lambdaRad))
	
	// Greenwich Mean Sidereal Time
	gmst := math.Mod(280.460+360.9856474*n, 360.0)
	
	// Local sidereal time
	lst := gmst + lon
	lstRad := lst * math.Pi / 180.0
	
	// Hour angle
	h := lstRad - alpha
	
	// Convert latitude to radians
	latRad := lat * math.Pi / 180.0
	
	// Calculate altitude
	sinAlt := math.Sin(latRad)*math.Sin(delta) + math.Cos(latRad)*math.Cos(delta)*math.Cos(h)
	altitude = math.Asin(sinAlt) * 180.0 / math.Pi
	
	// Calculate azimuth
	cosAz := (math.Sin(delta) - math.Sin(latRad)*sinAlt) / (math.Cos(latRad) * math.Cos(math.Asin(sinAlt)))
	azimuth = math.Acos(cosAz) * 180.0 / math.Pi
	
	if math.Sin(h) > 0 {
		azimuth = 360.0 - azimuth
	}
	
	return altitude, azimuth
}

// calculateSunriseSunset calculates sunrise and sunset times for the given date
func calculateSunriseSunset(t time.Time, lat, lon float64) (sunrise, sunset time.Time) {
	// Use civil twilight (-6 degrees)
	zenith := 90.833
	
	// Always calculate for today in local time
	local := t.Local()
	year, month, day := local.Date()
	dayOfYear := local.YearDay()
	
	// Approximate times
	lngHour := lon / 15.0
	
	// Sunrise
	tSunrise := float64(dayOfYear) + ((6.0 - lngHour) / 24.0)
	
	// Sun's mean anomaly
	M := (0.9856 * tSunrise) - 3.289
	
	// Sun's true longitude
	L := math.Mod(M+(1.916*math.Sin(M*math.Pi/180.0))+(0.020*math.Sin(2*M*math.Pi/180.0))+282.634, 360.0)
	
	// Sun's right ascension
	RA := math.Mod(math.Atan(0.91764*math.Tan(L*math.Pi/180.0))*180.0/math.Pi, 360.0)
	
	// Right ascension value needs to be in the same quadrant as L
	Lquadrant := math.Floor(L / 90.0) * 90.0
	RAquadrant := math.Floor(RA / 90.0) * 90.0
	RA = RA + (Lquadrant - RAquadrant)
	RA = RA / 15.0
	
	// Sun's declination
	sinDec := 0.39782 * math.Sin(L*math.Pi/180.0)
	cosDec := math.Cos(math.Asin(sinDec))
	
	// Sun's local hour angle
	cosH := (math.Cos(zenith*math.Pi/180.0) - (sinDec * math.Sin(lat*math.Pi/180.0))) / (cosDec * math.Cos(lat*math.Pi/180.0))
	
	if cosH > 1 {
		// Sun never rises
		return time.Time{}, time.Time{}
	}
	if cosH < -1 {
		// Sun never sets
		return time.Time{}, time.Time{}
	}
	
	H := 360.0 - (math.Acos(cosH) * 180.0 / math.Pi)
	H = H / 15.0
	
	// Local mean time of rising
	T := H + RA - (0.06571 * tSunrise) - 6.622
	
	// Adjust to UTC
	UT := math.Mod(T-lngHour, 24.0)
	if UT < 0 {
		UT += 24.0
	}
	
	// Convert to time
	hours := int(UT)
	minutes := int((UT - float64(hours)) * 60.0)
	sunrise = time.Date(year, month, day, hours, minutes, 0, 0, time.UTC).Local()
	
	// Sunset calculation (similar but with different hour angle)
	H = (math.Acos(cosH) * 180.0 / math.Pi) / 15.0
	T = H + RA - (0.06571 * tSunrise) - 6.622
	UT = math.Mod(T-lngHour, 24.0)
	if UT < 0 {
		UT += 24.0
	}
	
	hours = int(UT)
	minutes = int((UT - float64(hours)) * 60.0)
	sunset = time.Date(year, month, day, hours, minutes, 0, 0, time.UTC)
	
	// If sunset UTC hour is less than sunrise UTC hour, it's the next day in UTC
	if sunset.Before(sunrise) {
		sunset = sunset.Add(24 * time.Hour)
	}
	sunset = sunset.Local()
	
	return sunrise, sunset
}
