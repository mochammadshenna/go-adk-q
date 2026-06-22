package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// SearchHotels queries the central database for hotels matching criteria.
func (p *Pool) SearchHotels(ctx context.Context, params SearchParams) ([]HotelRow, int, error) {
	baseFrom := `FROM tb_hotels h
		JOIN tb_brands b ON h.brand_id = b.brand_id
		LEFT JOIN tb_region r ON h.region_id = r.region_id
		WHERE h.hotel_status = 1`

	var wheres []string
	args := []any{}

	if params.City != "" {
		// Also check hotel_address — "Bali" matches hotels in Kuta/Seminyak/Denpasar
		// whose address contains the province/island name.
		wheres = append(wheres, `(LOWER(r.region_name) LIKE LOWER(CONCAT('%', ?, '%'))
			OR LOWER(h.hotel_address) LIKE LOWER(CONCAT('%', ?, '%')))`)
		args = append(args, params.City, params.City)
	}
	if params.Country != "" && params.Country != "Indonesia" {
		wheres = append(wheres, "LOWER(r.region_name) LIKE LOWER(CONCAT('%', ?, '%'))")
		args = append(args, params.Country)
	}
	if params.Brand != "" {
		wheres = append(wheres, "LOWER(b.brand_name) LIKE LOWER(CONCAT('%', ?, '%'))")
		args = append(args, params.Brand)
	}
	if params.Query != "" {
		wheres = append(wheres, `(LOWER(h.hotel_name) LIKE LOWER(CONCAT('%', ?, '%'))
			OR LOWER(b.brand_name) LIKE LOWER(CONCAT('%', ?, '%')))`)
		args = append(args, params.Query, params.Query)
	}

	whereClause := ""
	for _, w := range wheres {
		whereClause += " AND " + w
	}

	// Count total matching hotels.
	var total int
	countSQL := "SELECT COUNT(*) " + baseFrom + whereClause
	if err := p.central.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}

	// Select matching hotels.
	selectCols := `SELECT
		h.hotel_id, h.api_hotel_id, h.brand_id,
		COALESCE(b.brand_name, ''),
		COALESCE(b.db_prefix_name, ''),
		COALESCE(r.region_name, ''),
		COALESCE(h.hotel_name, ''),
		COALESCE(h.hotel_address, ''),
		COALESCE(h.hotel_rating, 0),
		0,
		COALESCE(h.latitude, ''),
		COALESCE(h.longtitude, ''),
		COALESCE(h.hotel_starting_price, 0),
		COALESCE(h.hotel_currency, 'IDR'),
		COALESCE(b.brand_color, '')`

	query := selectCols + " " + baseFrom + whereClause
	query += " ORDER BY h.hotel_name ASC"
	if params.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", params.Limit)
	}
	if params.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", params.Offset)
	}

	rows, err := p.central.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var hotels []HotelRow
	for rows.Next() {
		h, err := scanHotel(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		hotels = append(hotels, h)
	}
	if hotels == nil {
		hotels = []HotelRow{}
	}
	return hotels, total, rows.Err()
}

// GetHotelByID fetches a single hotel by the central database hotel_id.
func (p *Pool) GetHotelByID(ctx context.Context, hotelID int) (*HotelRow, error) {
	query := `
		SELECT
			h.hotel_id, h.api_hotel_id, h.brand_id,
			COALESCE(b.brand_name, ''),
			COALESCE(b.db_prefix_name, ''),
			COALESCE(r.region_name, ''),
			COALESCE(h.hotel_name, ''),
			COALESCE(h.hotel_address, ''),
			COALESCE(h.hotel_rating, 0),
			0,
			COALESCE(h.latitude, ''),
			COALESCE(h.longtitude, ''),
			COALESCE(h.hotel_starting_price, 0),
			COALESCE(h.hotel_currency, 'IDR'),
			COALESCE(b.brand_color, '')
		FROM tb_hotels h
		JOIN tb_brands b ON h.brand_id = b.brand_id
		LEFT JOIN tb_region r ON h.region_id = r.region_id
		WHERE h.hotel_id = ? AND h.hotel_status = 1`
	return scanSingleHotel(p.central.QueryRowContext(ctx, query, hotelID))
}

// GetHotelByAPIID fetches a hotel by its api_hotel_id (the key to brand DBs).
func (p *Pool) GetHotelByAPIID(ctx context.Context, apiHotelID int) (*HotelRow, error) {
	query := `
		SELECT
			h.hotel_id, h.api_hotel_id, h.brand_id,
			COALESCE(b.brand_name, ''),
			COALESCE(b.db_prefix_name, ''),
			COALESCE(r.region_name, ''),
			COALESCE(h.hotel_name, ''),
			COALESCE(h.hotel_address, ''),
			COALESCE(h.hotel_rating, 0),
			0,
			COALESCE(h.latitude, ''),
			COALESCE(h.longtitude, ''),
			COALESCE(h.hotel_starting_price, 0),
			COALESCE(h.hotel_currency, 'IDR'),
			COALESCE(b.brand_color, '')
		FROM tb_hotels h
		JOIN tb_brands b ON h.brand_id = b.brand_id
		LEFT JOIN tb_region r ON h.region_id = r.region_id
		WHERE h.api_hotel_id = ? AND h.hotel_status = 1
		LIMIT 1`
	return scanSingleHotel(p.central.QueryRowContext(ctx, query, apiHotelID))
}

// ListRegions returns distinct region names for dashboard filters.
func (p *Pool) ListRegions(ctx context.Context) ([]string, error) {
	rows, err := p.central.QueryContext(ctx,
		`SELECT DISTINCT r.region_name
		 FROM tb_hotels h
		 JOIN tb_region r ON h.region_id = r.region_id
		 WHERE h.hotel_status = 1 AND r.region_name != ''
		 ORDER BY r.region_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var regions []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		regions = append(regions, name)
	}
	if regions == nil {
		regions = []string{}
	}
	return regions, rows.Err()
}

// HotelCount reports the total number of active hotels.
func (p *Pool) HotelCount(ctx context.Context) (int, error) {
	var n int
	err := p.central.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tb_hotels WHERE hotel_status = 1`).Scan(&n)
	return n, err
}

// GetThumbnails fetches thumbnail_desktop URLs from brand databases, then proxies each
// image to a base64 data URI so Claude Desktop's iframe CSP cannot block them.
// Hotels with no URL, unreachable brand DB, or images > 200 KB are omitted.
func (p *Pool) GetThumbnails(ctx context.Context, hotels []HotelRow) map[int]string {
	type entry struct{ centralID, apiID int }
	byPrefix := make(map[string][]entry)
	for _, h := range hotels {
		if h.DBPrefix == "" || !h.APIHotelID.Valid {
			continue
		}
		byPrefix[h.DBPrefix] = append(byPrefix[h.DBPrefix], entry{h.HotelID, int(h.APIHotelID.Int64)})
	}

	// Phase 1: collect CDN URLs from brand DBs.
	urls := make(map[int]string, len(hotels))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for prefix, entries := range byPrefix {
		prefix, entries := prefix, entries
		wg.Add(1)
		go func() {
			defer wg.Done()
			db := p.BrandDB(ctx, prefix)
			if db == nil || !p.HasColumn(prefix, "tb_hotels", "thumbnail_desktop") {
				return
			}
			ids := make([]any, len(entries))
			apiToCenter := make(map[int]int, len(entries))
			for i, e := range entries {
				ids[i] = e.apiID
				apiToCenter[e.apiID] = e.centralID
			}
			ph := strings.Repeat("?,", len(ids))
			ph = ph[:len(ph)-1]
			rows, err := db.QueryContext(ctx,
				"SELECT hotel_id, thumbnail_desktop FROM tb_hotels WHERE hotel_id IN ("+ph+") AND thumbnail_desktop IS NOT NULL AND thumbnail_desktop != ''",
				ids...)
			if err != nil {
				return
			}
			defer rows.Close()
			mu.Lock()
			defer mu.Unlock()
			for rows.Next() {
				var apiID int
				var thumb string
				if rows.Scan(&apiID, &thumb) == nil {
					if cid, ok := apiToCenter[apiID]; ok {
						urls[cid] = thumb
					}
				}
			}
		}()
	}
	wg.Wait()

	// Phase 2: rewrite CDN URLs through the Archipelago image resizer (pure string transform).
	result := make(map[int]string, len(urls))
	for cid, thumbURL := range urls {
		result[cid] = resizeImageURL(thumbURL, 0, 0, "center")
	}
	return result
}

// resizeImageURL rewrites a brand CDN URL through the Archipelago image resizer.
// Ported from ResizeImage in the Sentec platform codebase.
// Base URL from env url_image_resizer; defaults to https://images.archipelagohotels.com/.
func resizeImageURL(img string, width, height int, location string) string {
	if img == "" {
		return ""
	}
	urlImage := os.Getenv("url_image_resizer")
	if urlImage == "" {
		urlImage = "https://images.archipelagohotels.com/"
	}
	bucketName := ""
	r := regexp.MustCompile(`^(?:https?://)?(?:www\.)?([^/]+)`)
	matches := r.FindStringSubmatch(img)
	if len(matches) >= 2 {
		urls := strings.Split(matches[1], ".")
		if urls[0] == "sentineltech" {
			bucketName = "sentineltech-publicwebsite"
		} else if len(urls) >= 2 {
			bucketName = urls[1]
		}
	}
	baseURL := urlImage + bucketName + "/"
	cdn := strings.Split(img, ".")
	if len(cdn) < 2 {
		return img
	}
	trim := strings.Replace(img, cdn[0]+"."+cdn[1]+"."+"com/", "", 1)
	var link string
	if width == 0 && height == 0 {
		link = baseURL + trim
	} else if width != 0 && height == 0 {
		link = baseURL + trim + "?s=" + fmt.Sprint(width) + "&location=" + location
	} else if height == 0 || width == 0 {
		link = baseURL + trim + "?location=" + location
	} else {
		link = baseURL + trim + "?d=" + fmt.Sprintf("%dx%d", width, height) + "&location=" + location
	}
	return link
}

// scanHotel scans a HotelRow from a single result row.
func scanHotel(scanner interface {
	Scan(dest ...any) error
}) (HotelRow, error) {
	var h HotelRow
	var latStr, lngStr sql.NullString
	err := scanner.Scan(
		&h.HotelID, &h.APIHotelID, &h.BrandID,
		&h.BrandName, &h.DBPrefix,
		&h.RegionName,
		&h.Name, &h.Address,
		&h.Rating, &h.Stars,
		&latStr, &lngStr,
		&h.StartingPrice, &h.Currency,
		&h.BrandColor,
	)
	if err != nil {
		return h, err
	}
	if latStr.Valid {
		h.Latitude = parseFloat(latStr.String)
	}
	if lngStr.Valid {
		h.Longitude = parseFloat(lngStr.String)
	}
	h.ImageStyle = brandImageStyle(h.BrandName)
	// Derive star rating from guest rating since hotel_stars column doesn't exist in schema.
	if h.Stars == 0 && h.Rating > 0 {
		switch {
		case h.Rating >= 9.0:
			h.Stars = 5
		case h.Rating >= 8.0:
			h.Stars = 4
		case h.Rating >= 7.0:
			h.Stars = 3
		case h.Rating >= 6.0:
			h.Stars = 2
		default:
			h.Stars = 1
		}
	}
	return h, nil
}

func scanSingleHotel(row *sql.Row) (*HotelRow, error) {
	h, err := scanHotel(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return &h, nil
}
