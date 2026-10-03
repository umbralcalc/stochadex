package solarfleet

// Deterministic solar geometry: position, clear-sky irradiance, plane-of-array.
//
// Pure functions of (time, site metadata) — no state, no draws, no data. This is
// the deterministic physical backbone of the fleet model; the stochastic clear-sky
// index multiplies its output downstream. Lifted from the downstream repo's
// solarfleet/geometry.py (numpy), which validates it against published astronomy.
//
// Conventions: angles are degrees; azimuth is clockwise from north (north=0,
// east=90, south=180, west=270), matching NOAA and the OCF uk_pv metadata. Solar
// altitude is the geometric angle above the horizon (no refraction). Solar
// position follows the NOAA Solar Calculator algorithm (accurate to ~0.01 deg over
// 1900-2100), far finer than any error the stochastic layer introduces.

import (
	"math"
	"time"
)

const (
	// AExtraterrestrial is the apparent extraterrestrial normal irradiance of the
	// Meinel clear-sky form, W/m^2. Deliberately Meinel A = 1353 (not the modern
	// solar constant 1361): the 0.7 transmittance and the 1353 are a matched pair.
	AExtraterrestrial = 1353.0

	// IStandardTestConditions is the STC reference plane-of-array irradiance,
	// W/m^2. Panel nameplate kWp is defined here, so generation scales as poa/IStc.
	IStandardTestConditions = 1000.0

	// jdUnixEpoch is the Julian Date of the Unix epoch (1970-01-01T00:00:00 UTC).
	jdUnixEpoch = 2440587.5
)

func radians(deg float64) float64 { return deg * math.Pi / 180.0 }
func degrees(rad float64) float64 { return rad * 180.0 / math.Pi }

// solarParams returns the NOAA intermediate quantities shared by declination,
// equation of time, and position: (declination [rad], equation of time [min],
// seconds since UTC midnight). Ported from geometry.py::_solar_params.
func solarParams(t time.Time) (declRad, eqTimeMin, secsSinceMidnight float64) {
	unixS := float64(t.UTC().UnixNano()) / 1e9
	jd := unixS/86400.0 + jdUnixEpoch
	jc := (jd - 2451545.0) / 36525.0 // Julian centuries since J2000.0

	// Geometric mean longitude and anomaly of the sun (degrees).
	l0 := math.Mod(280.46646+jc*(36000.76983+jc*0.0003032), 360.0)
	m := 357.52911 + jc*(35999.05029-0.0001537*jc)
	mRad := radians(m)

	// Eccentricity of Earth's orbit and the equation of centre.
	e := 0.016708634 - jc*(0.000042037+0.0000001267*jc)
	c := math.Sin(mRad)*(1.914602-jc*(0.004817+0.000014*jc)) +
		math.Sin(2*mRad)*(0.019993-0.000101*jc) +
		math.Sin(3*mRad)*0.000289
	trueLong := l0 + c

	// Apparent longitude (nutation + aberration correction).
	omega := 125.04 - 1934.136*jc
	appLong := trueLong - 0.00569 - 0.00478*math.Sin(radians(omega))

	// Obliquity of the ecliptic, with the small correction term.
	seconds := 21.448 - jc*(46.815+jc*(0.00059-jc*0.001813))
	meanObliq := 23.0 + (26.0+seconds/60.0)/60.0
	obliqCorr := meanObliq + 0.00256*math.Cos(radians(omega))
	obliqRad := radians(obliqCorr)

	declRad = math.Asin(math.Sin(obliqRad) * math.Sin(radians(appLong)))

	// Equation of time (minutes).
	y := math.Pow(math.Tan(obliqRad/2.0), 2)
	l0Rad := radians(l0)
	eqTimeMin = 4.0 * degrees(
		y*math.Sin(2*l0Rad)-
			2*e*math.Sin(mRad)+
			4*e*y*math.Sin(mRad)*math.Cos(2*l0Rad)-
			0.5*y*y*math.Sin(4*l0Rad)-
			1.25*e*e*math.Sin(2*mRad))

	secsSinceMidnight = math.Mod(unixS, 86400.0)
	return declRad, eqTimeMin, secsSinceMidnight
}

// SolarPosition returns solar altitude and azimuth in degrees (geometric, no
// refraction; altitude may be negative at night; azimuth clockwise from north in
// [0,360)). Ported from geometry.py::solar_position.
func SolarPosition(latitude, longitude float64, t time.Time) (altitudeDeg, azimuthDeg float64) {
	declRad, eqTime, secs := solarParams(t)
	latRad := radians(latitude)

	// True solar time (minutes) and hour angle (degrees). UTC => timezone 0.
	minutesUTC := secs / 60.0
	trueSolarTime := math.Mod(minutesUTC+eqTime+4.0*longitude, 1440.0)
	hourAngle := trueSolarTime/4.0 - 180.0 // degrees, 0 at local solar noon
	haRad := radians(hourAngle)

	cosZenith := math.Sin(latRad)*math.Sin(declRad) +
		math.Cos(latRad)*math.Cos(declRad)*math.Cos(haRad)
	cosZenith = math.Max(-1.0, math.Min(1.0, cosZenith))
	zenithRad := math.Acos(cosZenith)
	altitudeDeg = 90.0 - degrees(zenithRad)

	// Azimuth via the NOAA branch on the hour-angle sign.
	sinZenith := math.Sin(zenithRad)
	cosAz := 0.0
	if sinZenith > 1e-9 {
		cosAz = (math.Sin(latRad)*cosZenith - math.Sin(declRad)) /
			(math.Cos(latRad) * sinZenith)
	}
	cosAz = math.Max(-1.0, math.Min(1.0, cosAz))
	azCore := degrees(math.Acos(cosAz))
	if hourAngle > 0.0 {
		azimuthDeg = math.Mod(azCore+180.0, 360.0)
	} else {
		azimuthDeg = math.Mod(540.0-azCore, 360.0)
	}
	return altitudeDeg, azimuthDeg
}

// ClearSkyNormalIrradiance is the clear-sky normal (beam) irradiance, W/m^2
// (Meinel / Heliodon form): I_csn(h) = A * 0.7^((1/sin h)^0.678) for 0 < h < 90,
// else 0. Ported from geometry.py::clear_sky_normal_irradiance.
func ClearSkyNormalIrradiance(altitudeDeg float64) float64 {
	sinH := math.Sin(radians(altitudeDeg))
	if sinH <= 0.0 {
		return 0.0 // sun below the horizon; air mass 1/sin h blows up at h=0
	}
	airMass := 1.0 / sinH
	return AExtraterrestrial * math.Pow(0.7, math.Pow(airMass, 0.678))
}

// PoaCosIncidence is the cosine of the beam incidence angle on a tilted plane,
// clipped at 0: cos(theta) = sin(tilt) cos(h) cos(zeta - gamma) + cos(tilt) sin(h),
// with h = altitude, gamma = solar azimuth, zeta = surface azimuth, tilt = surface
// tilt from horizontal. Ported from geometry.py::poa_cos_incidence.
func PoaCosIncidence(altitudeDeg, azimuthDeg, tiltDeg, surfaceAzimuthDeg float64) float64 {
	h := radians(altitudeDeg)
	gamma := radians(azimuthDeg)
	tilt := radians(tiltDeg)
	zeta := radians(surfaceAzimuthDeg)
	cosInc := math.Sin(tilt)*math.Cos(h)*math.Cos(zeta-gamma) + math.Cos(tilt)*math.Sin(h)
	if cosInc < 0.0 {
		return 0.0
	}
	return cosInc
}

// ClearSkyPOA is the clear-sky plane-of-array (beam) irradiance for a site, W/m^2:
// it composes SolarPosition, ClearSkyNormalIrradiance and PoaCosIncidence, and is
// zero whenever the sun is below the horizon or behind the panel. Ported from
// geometry.py::clear_sky_poa.
func ClearSkyPOA(latitude, longitude float64, t time.Time, tiltDeg, surfaceAzimuthDeg float64) float64 {
	altitude, azimuth := SolarPosition(latitude, longitude, t)
	dni := ClearSkyNormalIrradiance(altitude)
	cosInc := PoaCosIncidence(altitude, azimuth, tiltDeg, surfaceAzimuthDeg)
	return dni * cosInc
}
