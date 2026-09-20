package database

import "time"

var ISTLocation *time.Location

func init() {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		// Fallback fixed zone if system tzdata is missing in slim containers
		ISTLocation = time.FixedZone("IST", 5*3600+30*60)
		return
	}
	ISTLocation = loc
}

// NowIST returns the current time strictly in IST
func NowIST() time.Time {
	return time.Now().In(ISTLocation)
}

// ToIST converts any time.Time instance to IST
func ToIST(t time.Time) time.Time {
	return t.In(ISTLocation)
}

// FormatIST returns ISO-8601 formatted timestamp with IST offset
func FormatIST(t time.Time) string {
	return t.In(ISTLocation).Format("2006-01-02T15:04:05-07:00")
}
