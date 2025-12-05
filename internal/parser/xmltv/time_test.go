package xmltv

import (
	"encoding/xml"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTime_MarshalXMLAttr(t *testing.T) {
	timestamp := time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC)
	timeObj := Time{Time: timestamp}

	attr, err := timeObj.MarshalXMLAttr(xml.Name{Local: "start"})
	require.NoError(t, err)
	assert.Equal(t, "start", attr.Name.Local)
	assert.Equal(t, "20230101123045 +0000", attr.Value)
}

func TestTime_UnmarshalXMLAttr(t *testing.T) {
	tests := []struct {
		name      string
		attrValue string
		expected  time.Time
		err       bool
	}{
		{
			name:      "Standard format with timezone",
			attrValue: "20230101123045 -0700",
			expected:  time.Date(2023, 1, 1, 12, 30, 45, 0, time.FixedZone("", -7*60*60)),
			err:       false,
		},
		{
			name:      "Without timezone",
			attrValue: "20230101123045",
			expected:  time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC),
			err:       false,
		},
		{
			name:      "With +0000 timezone",
			attrValue: "20230101123045 +0000",
			expected:  time.Date(2023, 1, 1, 12, 30, 45, 0, time.UTC),
			err:       false,
		},
		{
			name:      "Negative value",
			attrValue: "-20230101123045",
			expected:  time.Time{},
			err:       false,
		},
		{
			name:      "Invalid format",
			attrValue: "invalid",
			expected:  time.Time{},
			err:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeObj := Time{}
			err := timeObj.UnmarshalXMLAttr(xml.Attr{Value: tt.attrValue})

			if tt.err {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected.Format(time.RFC3339), timeObj.Time.Format(time.RFC3339))
		})
	}
}

func TestDate_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlValue string
		expected time.Time
		err      bool
	}{
		{
			name:     "Full timestamp with timezone",
			xmlValue: "<TestStruct><date>20100401170000 +0000</date></TestStruct>",
			expected: time.Date(2010, 4, 1, 17, 0, 0, 0, time.UTC),
			err:      false,
		},
		{
			name:     "Full timestamp without timezone",
			xmlValue: "<TestStruct><date>20230101120000</date></TestStruct>",
			expected: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
			err:      false,
		},
		{
			name:     "Date only YYYYMMDD",
			xmlValue: "<TestStruct><date>20230615</date></TestStruct>",
			expected: time.Date(2023, 6, 15, 0, 0, 0, 0, time.UTC),
			err:      false,
		},
		{
			name:     "Year only",
			xmlValue: "<TestStruct><date>2023</date></TestStruct>",
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			err:      false,
		},
		{
			name:     "Pipe-separated value",
			xmlValue: "<TestStruct><date>2023|extra</date></TestStruct>",
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
			err:      false,
		},
		{
			name:     "Invalid format",
			xmlValue: "<TestStruct><date>invalid</date></TestStruct>",
			expected: time.Time{},
			err:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type TestStruct struct {
				XMLName xml.Name `xml:"TestStruct"`
				Date    Date     `xml:"date"`
			}

			var result TestStruct
			err := xml.Unmarshal([]byte(tt.xmlValue), &result)

			if tt.err {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			actual := time.Time(result.Date)
			assert.Equal(t, tt.expected.Format(time.RFC3339), actual.Format(time.RFC3339))
		})
	}
}

func TestElementPresent_MarshalUnmarshalXML(t *testing.T) {
	elPresent := ElementPresent{Present: true}

	type TestStruct struct {
		New *ElementPresent `xml:"new"`
	}

	ts := TestStruct{New: &elPresent}
	xmlData, err := xml.Marshal(ts)
	require.NoError(t, err)
	assert.Contains(t, string(xmlData), "<new></new>")

	elNotPresent := ElementPresent{Present: false}
	ts.New = &elNotPresent
	xmlData, err = xml.Marshal(ts)
	require.NoError(t, err)
	assert.NotContains(t, string(xmlData), "<new>")

	var result TestStruct
	xmlStr := "<TestStruct><new></new></TestStruct>"
	err = xml.Unmarshal([]byte(xmlStr), &result)
	require.NoError(t, err)
	assert.NotNil(t, result.New)
	assert.True(t, result.New.Present)
}
