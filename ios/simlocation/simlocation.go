package simlocation

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	ios "github.com/danielpaulus/go-ios/ios"
	log "github.com/sirupsen/logrus"
)

const serviceName string = "com.apple.dt.simulatelocation"

type Connection struct {
	deviceConn ios.DeviceConnectionInterface
	plistCodec ios.PlistCodec
}

type locationData struct {
	lon float64
	lat float64
}

func New(device ios.DeviceEntry) (*Connection, error) {
	locationConn, err := ios.ConnectToService(device, serviceName)
	if err != nil {
		return &Connection{}, err
	}
	return &Connection{deviceConn: locationConn, plistCodec: ios.NewPlistCodec()}, nil
}

func (locationConn *Connection) Close() {
	locationConn.deviceConn.Close()
}

// Set the device location to a point by latitude and longitude
func SetLocation(device ios.DeviceEntry, lat string, lon string) error {
	if lat == "" || lon == "" {
		return errors.New("Please provide non-empty values for latitude and longitude")
	}

	// Create new connection to the location service
	locationConn, err := New(device)
	if err != nil {
		return err
	}

	latitude, err := strconv.ParseFloat(lat, 64)
	if err != nil {
		return err
	}

	longitude, err := strconv.ParseFloat(lon, 64)
	if err != nil {
		return err
	}

	data := new(locationData)
	data.lat = latitude
	data.lon = longitude

	log.WithFields(log.Fields{"latitude": latitude, "longitude": longitude}).
		Info("Simulating device location")

	// Generate the byte data needed by the service to set the location
	locationBytes, err := data.LocationBytes()
	if err != nil {
		return err
	}

	// Send the generated byte data for the expected simulated coordinates
	err = locationConn.deviceConn.Send(locationBytes)
	if err != nil {
		return err
	}

	locationConn.Close()
	return nil
}

type Gpx struct {
	XMLName xml.Name `xml:"gpx"`
	Tracks  []Track  `xml:"trk"`
}

type Track struct {
	XMLName       xml.Name       `xml:"trk"`
	TrackSegments []TrackSegment `xml:"trkseg"`
	Name          string         `xml:"name"`
}

type TrackSegment struct {
	XMLName     xml.Name     `xml:"trkseg"`
	TrackPoints []TrackPoint `xml:"trkpt"`
}

type TrackPoint struct {
	XMLName        xml.Name `xml:"trkpt"`
	PointLongitude string   `xml:"lon,attr"`
	PointLatitude  string   `xml:"lat,attr"`
	PointTime      string   `xml:"time"`
}

// Simulate live tracking using a gpx file
func SetLocationGPX(device ios.DeviceEntry, filePath string) error {
	// Try to open the gpx file by the provided file path
	gpxFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer gpxFile.Close()

	// Read the gpx file
	byteData, err := io.ReadAll(gpxFile)
	if err != nil {
		return err
	}

	// Parse the gpx file which should be a valid XML
	var gpx Gpx
	err = xml.Unmarshal(byteData, &gpx)
	if err != nil {
		return err
	}

	var lastPointTime time.Time

	// Loop through all available tracks, their segments and the segments respective track points to cover the whole file
	for _, track := range gpx.Tracks {
		for _, segment := range track.TrackSegments {
			for _, point := range segment.TrackPoints {

				// Parse the point time string to time.Time object
				currentPointTime, err := time.Parse(time.RFC3339, point.PointTime)

				// If there was a previous point covered
				// and if the difference between the last point time and the current point time is > 0 in seconds
				// we wait for this duration to simulate the actual gpx tracking
				if !lastPointTime.IsZero() {
					if err != nil {
						return err
					}

					// Get the duration between the current point time and the last point time in seconds
					duration := currentPointTime.Unix() - lastPointTime.Unix()

					// Sleep for the calculated duration in seconds before updating the location
					if duration > 0 {
						time.Sleep(time.Duration(duration) * time.Second)
					}
				}

				// Change the last point time to the time of the currently set point
				lastPointTime = currentPointTime
				pointLon := point.PointLongitude
				pointLat := point.PointLatitude

				// Set the current point location by its latitude and longitude
				err = SetLocation(device, pointLat, pointLon)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func ResetLocation(device ios.DeviceEntry) error {
	// Create a new connection to the location service
	locationConn, err := New(device)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)

	// The location service accepts the binary representation of 1 to reset to the original location
	err = binary.Write(buf, binary.BigEndian, uint32(1))
	if err != nil {
		return err
	}

	// Send the byte data that should reset the simulated location
	err = locationConn.deviceConn.Send(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}

// Create the byte data needed to set a specific location
func (l *locationData) LocationBytes() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, uint32(0)); err != nil {
		return nil, fmt.Errorf("creating location bytes: %w", err)
	}

	latString := fmt.Sprintf("%f", l.lat)
	latBytes := []byte(latString)
	if err := binary.Write(buf, binary.BigEndian, uint32(len(latBytes))); err != nil {
		return nil, fmt.Errorf("creating location bytes: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, latBytes); err != nil {
		return nil, fmt.Errorf("creating location bytes: %w", err)
	}

	lonString := fmt.Sprintf("%f", l.lon)
	lonBytes := []byte(lonString)
	if err := binary.Write(buf, binary.BigEndian, uint32(len(lonBytes))); err != nil {
		return nil, fmt.Errorf("creating location bytes: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, lonBytes); err != nil {
		return nil, fmt.Errorf("creating location bytes: %w", err)
	}

	return buf.Bytes(), nil
}
