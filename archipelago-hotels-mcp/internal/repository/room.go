package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// GetRooms fetches the active room types for a hotel from its per-brand database.
// Routes to tb_hrooms (standard) or tb_hroom (PBA) depending on brand.
func (p *Pool) GetRooms(ctx context.Context, brandPrefix string, apiHotelID int) ([]RoomRow, error) {
	brandDB := p.BrandDB(ctx, brandPrefix)
	if brandDB == nil {
		return nil, nil
	}

	table := "tb_hrooms"
	statusCol := "room_status"
	statusVal := "Y"

	// PBA uses a different table and column naming.
	if brandPrefix == "pba" {
		table = "tb_hroom"
		statusCol = "status"
		statusVal = "1"
	}

	// Build SELECT columns, accounting for schema differences.
	cols := []string{"room_name"}
	if p.HasColumn(brandPrefix, table, "room_rate") {
		cols = append(cols, "COALESCE(room_rate, 0)")
	} else {
		cols = append(cols, "0")
	}
	if p.HasColumn(brandPrefix, table, "sb_id") {
		cols = append(cols, "sb_id")
	} else {
		cols = append(cols, "NULL")
	}
	if p.HasColumn(brandPrefix, table, statusCol) {
		cols = append(cols, statusCol)
	} else {
		cols = append(cols, "'Y'")
	}
	if p.HasColumn(brandPrefix, table, "sentec_id") {
		cols = append(cols, "sentec_id")
	} else {
		cols = append(cols, "NULL")
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE hotel_id = ? AND %s = ? ORDER BY room_name`,
		strings.Join(cols, ", "), table, statusCol)

	rows, err := brandDB.QueryContext(ctx, query, apiHotelID, statusVal)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var rooms []RoomRow
	for rows.Next() {
		room, err := scanRoom(rows)
		if err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		rooms = append(rooms, room)
	}
	if rooms == nil {
		rooms = []RoomRow{}
	}
	return rooms, rows.Err()
}

// scanRoom scans a RoomRow, handling both DECIMAL and INT room_rate types.
func scanRoom(scanner interface{ Scan(dest ...any) error }) (RoomRow, error) {
	var r RoomRow
	var rateFloat sql.NullFloat64
	cols := []any{&r.Name, &rateFloat, &r.SBID, &r.Status, &r.SentecID}
	if err := scanner.Scan(cols...); err != nil {
		// Some DBs store room_rate as INT rather than DECIMAL; retry.
		if strings.Contains(err.Error(), "converting") {
			var rateInt sql.NullInt64
			alt := []any{&r.Name, &rateInt, &r.SBID, &r.Status, &r.SentecID}
			if err2 := scanner.Scan(alt...); err2 == nil && rateInt.Valid {
				r.Rate = float64(rateInt.Int64)
				return r, nil
			}
		}
		return RoomRow{}, err
	}
	if rateFloat.Valid {
		r.Rate = rateFloat.Float64
	}
	return r, nil
}

// GetCredentials fetches booking credentials for a hotel from its brand DB.
func (p *Pool) GetCredentials(ctx context.Context, brandPrefix string, apiHotelID int) (*BrandCredentials, error) {
	brandDB := p.BrandDB(ctx, brandPrefix)
	if brandDB == nil {
		return nil, nil
	}

	// Discover available columns.
	type colInfo struct{ name, alias string }
	var cols []colInfo
	for _, c := range []string{"simplebooking_id", "simplebooking_user", "simplebooking_pass",
		"xml_user", "xml_pass", "hotel_channel", "sentec_booking_id"} {
		if p.HasColumn(brandPrefix, "tb_hotels", c) {
			cols = append(cols, colInfo{name: c, alias: c})
		}
	}
	if len(cols) == 0 {
		return nil, nil
	}

	sel := make([]string, len(cols))
	for i, c := range cols {
		sel[i] = c.name
	}

	query := fmt.Sprintf("SELECT %s FROM tb_hotels WHERE hotel_id = ?",
		strings.Join(sel, ", "))
	row := brandDB.QueryRowContext(ctx, query, apiHotelID)

	creds := &BrandCredentials{}
	targets := make([]any, len(cols))
	for i, c := range cols {
		switch c.name {
		case "simplebooking_id":
			targets[i] = &creds.SimpleBookingID
		case "simplebooking_user":
			targets[i] = &creds.SimpleBookingUser
		case "simplebooking_pass":
			targets[i] = &creds.SimpleBookingPass
		case "xml_user":
			targets[i] = &creds.XMLUser
		case "xml_pass":
			targets[i] = &creds.XMLPass
		case "hotel_channel":
			targets[i] = &creds.HotelChannel
		case "sentec_booking_id":
			targets[i] = &creds.SentecBookingID
		default:
			var dummy string
			targets[i] = &dummy
		}
	}

	if err := row.Scan(targets...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scan: %w", err)
	}

	return creds, nil
}
