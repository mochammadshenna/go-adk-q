package rate

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Provider credentials — same across all hotels (hardcoded in PHP source).
const (
	providerUsername = "Xmsfttgad33"
	providerPassword = "XMLfegg423!.33"
)

// postXML sends an XML POST request and returns the raw response body.
// ponytail: plain http.Post with 10s timeout. No retry — circuit breaker at
// caller level handles retry.
func postXML(ctx context.Context, url, body string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/xml")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode != 200 {
		return raw, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	return raw, nil
}

// buildSBXML constructs the OTA_HotelAvailRQ XML request.
func buildSBXML(req SBRequest, provider, providerPwd string) string {
	var couponXML string
	if req.Username == "" || req.Password == "" || req.SBID == 0 {
		return ""
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<OTA_HotelAvailRQ PrimaryLangID="EN" xmlns="http://www.opentravel.org/OTA/2003/05" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" Target="Production" Version="1.0" xsi:schemaLocation="http://www.opentravel.org/OTA/2003/05">
  <AvailRequestSegments>
    <AvailRequestSegment>
      <StayDateRange Start="%s" End="%s"/>
%s
      <RoomStayCandidates>
        <RoomStayCandidate>
          <GuestCounts>
            <GuestCount AgeQualifyingCode="10.AQC" Count="2"/>
          </GuestCounts>
        </RoomStayCandidate>
      </RoomStayCandidates>
      <TPA_Extensions>
        <provider Name="%s" Pwd="%s"/>
        <XMLHotelAgent Name="%s" Pwd="%s"/>
        <Filter HotelCode="%d"/>
      </TPA_Extensions>
    </AvailRequestSegment>
  </AvailRequestSegments>
</OTA_HotelAvailRQ>`,
		req.StartDate, req.EndDate,
		couponXML,
		provider, providerPwd,
		req.Username, req.Password,
		req.SBID,
	)
}

// parseSBResponse parses the SimpleBooking XML response into room rates.
func parseSBResponse(rawXML []byte) ([]SBRate, error) {
	// Strip all xmlns declarations — encoding/xml can't handle them simply.
	re := regexp.MustCompile(`\s+xmlns[^=]*="[^"]*"`)
	clean := re.ReplaceAll(rawXML, []byte{})

	var resp OTAHotelAvailRS
	if err := xml.Unmarshal(clean, &resp); err != nil {
		return nil, fmt.Errorf("xml unmarshal: %w", err)
	}
	if resp.RoomStays == nil || len(resp.RoomStays.RoomStay) == 0 {
		return nil, nil
	}

	var rates []SBRate
	for _, rs := range resp.RoomStays.RoomStay {
		if rs.RoomRates == nil || len(rs.RoomRates.RoomRate) == 0 {
			continue
		}
		for _, rr := range rs.RoomRates.RoomRate {
			rate := parseRatesBlock(rr)
			if rate != nil {
				rates = append(rates, *rate)
			}
		}
	}
	return rates, nil
}

// parseRatesBlock extracts a single SBRate from a RoomRate XML element.
// Base.AmountBeforeTax = rack rate (before discount).
// Total.AmountAfterTax  = final price after discount.
func parseRatesBlock(rr RoomRateXML) *SBRate {
	if rr.Rates == nil || len(rr.Rates.Rate) == 0 {
		return nil
	}
	first := rr.Rates.Rate[0]
	beforeDiscount := parseAttr(first.Base.AmountBeforeTax)
	total := parseAttr(first.Total.AmountAfterTax)
	slog.Debug("sb rate fields", "room", rr.RoomName, "Base.BeforeTax", first.Base.AmountBeforeTax, "Base.AfterTax", first.Base.AmountAfterTax, "Total.AfterTax", first.Total.AmountAfterTax, "parsed_before", beforeDiscount, "parsed_total", total)
	if beforeDiscount == 0 && total == 0 {
		return nil
	}
	return &SBRate{
		RoomName:           rr.RoomName,
		RoomID:             rr.RoomTypeCode,
		BeforeDiscountRate: beforeDiscount,
		TotalAfterTax:      total,
	}
}

func parseAttr(s string) float64 {
	v := strings.TrimSpace(s)
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// ----- XML structs for SimpleBooking response -----

// OTAHotelAvailRS is the root response element.
type OTAHotelAvailRS struct {
	XMLName   xml.Name     `xml:"OTA_HotelAvailRS"`
	RoomStays *RoomStaysXML `xml:"RoomStays"`
}

type RoomStaysXML struct {
	RoomStay []RoomStayXML `xml:"RoomStay"`
}

type RoomStayXML struct {
	RoomRates *RoomRatesXML `xml:"RoomRates"`
	// RoomStay element may also have a RoomType child with RoomName.
	RoomType *struct {
		RoomName string `xml:"RoomName,attr"`
	} `xml:"RoomType"`
}

type RoomRatesXML struct {
	RoomRate []RoomRateXML `xml:"RoomRate"`
}

type RoomRateXML struct {
	RoomTypeCode string    `xml:"RoomTypeCode,attr"` // SB room ID — matches tb_hrooms.sb_id
	RoomName     string    `xml:",attr"`
	Rates        *RatesXML `xml:"Rates"`
}

type RatesXML struct {
	Rate []RateXML `xml:"Rate"`
}

type RateXML struct {
	Base  RateAmountXML `xml:"Base"`
	Total RateAmountXML `xml:"Total"`
}

type RateAmountXML struct {
	AmountBeforeTax string `xml:"AmountBeforeTax,attr"`
	AmountAfterTax  string `xml:"AmountAfterTax,attr"`
}
