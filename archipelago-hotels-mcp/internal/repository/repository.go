// Package repository implements the data access layer for Archipelago Hotels.
//
// It centralises all external data sources:
//   - Central MySQL (db_archipelagowebsite) — hotel catalog, brands, regions
//   - Per-brand MySQL databases — room types, booking credentials
//   - SimpleBooking XML API — live room rates
//   - Sentec REST API — live room rates (reserve)
//
// Each source is a separate file with its own repository type.
// Repositories are stateless — all state lives in the Pool connection manager.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Config holds MySQL connection parameters.
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

// DSN builds a MySQL DSN string.
func (c Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.DBName)
}

// BrandDSN returns a DSN for a per-brand database.
func (c Config) BrandDSN(dbName string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		c.User, c.Password, c.Host, c.Port, dbName)
}

// BrandRow is a single row from tb_brands.
type BrandRow struct {
	BrandID       int
	BrandName     string
	DBPrefixName  string
	ParentBrandID int
	BrandColor    string
}

// HotelRow is a hotel from the central database.
type HotelRow struct {
	HotelID       int
	APIHotelID    sql.NullInt64
	BrandID       int
	BrandName     string
	DBPrefix      string
	RegionName    string
	Name          string
	Address       string
	Rating        float64
	Stars         int
	Latitude      float64
	Longitude     float64
	StartingPrice float64
	Currency      string
	ImageStyle    string
	BrandColor    string
}

// RoomRow is a room type from a per-brand database.
type RoomRow struct {
	Name     string
	Rate     float64
	SBID     sql.NullInt64
	Status   string
	SentecID sql.NullInt64
}

// BrandCredentials holds booking engine credentials from a brand DB.
type BrandCredentials struct {
	SimpleBookingID   int
	SimpleBookingUser string
	SimpleBookingPass string
	XMLUser           string
	XMLPass           string
	HotelChannel      string
	SentecBookingID   sql.NullString
}

// SearchParams for hotel search queries.
type SearchParams struct {
	City    string
	Country string
	Brand   string
	Query   string
	Limit   int
	Offset  int
}

// Pool manages connections to the central database and per-brand databases.
// Per-brand connections are lazily initialised on first use.
type Pool struct {
	central          *sql.DB
	config           Config
	brandDBs         map[string]*sql.DB
	brandCols        map[string]map[string]map[string]bool // brandPrefix → table → column → true
	brands           map[int]BrandRow
	thumbnailDomains []string // CDN hostnames discovered from thumbnail_desktop at startup
	mu               sync.RWMutex
}

// SetThumbnailDomains stores CDN hostnames discovered from brand DB thumbnail URLs.
func (p *Pool) SetThumbnailDomains(domains []string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.thumbnailDomains = domains
	p.mu.Unlock()
}

// ImageDomains returns the combined list of image origins for the MCP iframe CSP.
// Returns full https:// origins — bare hostnames are not valid CSP source directives.
func (p *Pool) ImageDomains() []string {
	if p == nil {
		return []string{"https://images.archipelagohotels.com"}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, 1+len(p.thumbnailDomains))
	out = append(out, "https://images.archipelagohotels.com")
	for _, d := range p.thumbnailDomains {
		out = append(out, "https://"+d)
	}
	return out
}

// NewPool connects to the central database and caches brand metadata.
// Returns error only if the central DB is unreachable.
func NewPool(ctx context.Context, cfg Config) (*Pool, error) {
	central, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("central open: %w", err)
	}
	central.SetMaxOpenConns(10)
	central.SetMaxIdleConns(3)
	central.SetConnMaxLifetime(5 * time.Minute)

	if err := central.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("central ping: %w", err)
	}

	p := &Pool{
		central:  central,
		config:   cfg,
		brandDBs: make(map[string]*sql.DB),
		brandCols: make(map[string]map[string]map[string]bool),
	}

	if err := p.loadBrands(ctx); err != nil {
		slog.Warn("brand cache failed", "error", err)
	}

	return p, nil
}

// Close closes all open database connections.
func (p *Pool) Close() {
	p.central.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, db := range p.brandDBs {
		db.Close()
	}
}

// Central returns the central database connection.
func (p *Pool) Central() *sql.DB { return p.central }

// Health checks connectivity of all known databases.
func (p *Pool) Health(ctx context.Context) error {
	if err := p.central.PingContext(ctx); err != nil {
		return fmt.Errorf("central DB: %w", err)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for name, db := range p.brandDBs {
		if err := db.PingContext(ctx); err != nil {
			return fmt.Errorf("brand DB %s: %w", name, err)
		}
	}
	return nil
}

// BrandDB returns (or lazily connects) a per-brand database by its db_prefix_name.
// Caches both successful and failed connections to avoid repeated attempts.
// Returns nil if the brand database doesn't exist or is unreachable.
func (p *Pool) BrandDB(ctx context.Context, prefix string) *sql.DB {
	if prefix == "" {
		return nil
	}

	p.mu.RLock()
	db, ok := p.brandDBs[prefix]
	p.mu.RUnlock()
	if ok {
		return db
	}

	// Connect outside the lock (slow path: sql.Open + Ping).
	// Multiple goroutines may enter here for different prefixes — that's fine
	// because they connect different databases. The write lock below serializes
	// the map store and scanColumns.
	db = p.connectBrand(ctx, prefix)

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check: another goroutine may have stored this prefix while we connected.
	if existing, ok := p.brandDBs[prefix]; ok {
		if db != nil && db != existing {
			db.Close()
		}
		return existing
	}

	p.brandDBs[prefix] = db
	if db != nil {
		p.scanColumns(prefix, db)
	}
	return db
}

// brandDBName maps prefix differences between db_prefix_name and actual database name.
// ponytail: static map — not every brand DB follows the {prefix}website pattern.
var brandDBName = map[string]string{
	"favehotel": "db_favewebsite",
	"pba":       "db_pba",
}

func (p *Pool) connectBrand(ctx context.Context, prefix string) *sql.DB {
	dbName, ok := brandDBName[prefix]
	if !ok {
		dbName = "db_" + prefix + "website"
	}
	dsn := p.config.BrandDSN(dbName)

	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		slog.Warn("brand DB open failed", "prefix", prefix, "db", dbName, "error", err)
		return nil
	}
	conn.SetMaxOpenConns(3)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		slog.Debug("brand DB unreachable", "prefix", prefix, "db", dbName, "error", err)
		conn.Close()
		return nil
	}

	slog.Info("brand DB connected", "prefix", prefix, "db", dbName)
	return conn
}

// HasColumn reports whether a brand DB table has a given column.
func (p *Pool) HasColumn(prefix, table, column string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	tables, ok := p.brandCols[prefix]
	if !ok {
		return false
	}
	cols, ok := tables[table]
	if !ok {
		return false
	}
	return cols[column]
}

func (p *Pool) scanColumns(prefix string, db *sql.DB) {
	rows, err := db.Query(
		`SELECT TABLE_NAME, COLUMN_NAME
		 FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = (SELECT DATABASE())
		   AND TABLE_NAME IN ('tb_hotels','tb_hrooms','tb_hroom')`)
	if err != nil {
		slog.Warn("column scan failed", "prefix", prefix, "error", err)
		return
	}
	defer rows.Close()

	tables := make(map[string]map[string]bool)
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			continue
		}
		if tables[table] == nil {
			tables[table] = make(map[string]bool)
		}
		tables[table][column] = true
	}
	p.brandCols[prefix] = tables
}

// loadBrands caches the brand catalog on startup.
func (p *Pool) loadBrands(ctx context.Context) error {
	rows, err := p.central.QueryContext(ctx,
		`SELECT brand_id, brand_name, COALESCE(db_prefix_name,''), COALESCE(parent_brand_id,0), COALESCE(brand_color,'')
		 FROM tb_brands ORDER BY brand_id`)
	if err != nil {
		return err
	}
	defer rows.Close()

	p.brands = make(map[int]BrandRow)
	for rows.Next() {
		var b BrandRow
		if err := rows.Scan(&b.BrandID, &b.BrandName, &b.DBPrefixName, &b.ParentBrandID, &b.BrandColor); err != nil {
			return err
		}
		p.brands[b.BrandID] = b
	}
	return rows.Err()
}

// BrandPrefix returns the db_prefix_name for a brand, resolving any parent chain.
func (p *Pool) BrandPrefix(brandID int) string {
	b, ok := p.brands[brandID]
	if !ok {
		return ""
	}
	if b.ParentBrandID != 0 {
		if parent, ok := p.brands[b.ParentBrandID]; ok {
			return parent.DBPrefixName
		}
	}
	return b.DBPrefixName
}

// ConfigFromEnv reads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Host:     envOr("MYSQL_HOST", "127.0.0.1"),
		Port:     envOr("MYSQL_PORT", "3306"),
		User:     envOr("MYSQL_USER", "root"),
		Password: envOr("MYSQL_PASS", ""),
		DBName:   envOr("MYSQL_DB", "db_archipelagowebsite"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parseFloat safely converts a string to float64.
func parseFloat(s string) float64 {
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

// brandImageStyle returns a Tailwind gradient class for dashboard cards.
func brandImageStyle(brandName string) string {
	styles := map[string]string{
		"aston":          "bg-gradient-to-br from-blue-800 to-sky-700",
		"grand aston":    "bg-gradient-to-br from-indigo-900 to-purple-900",
		"the alana":      "bg-gradient-to-br from-emerald-800 to-teal-700",
		"favehotels":     "bg-gradient-to-br from-pink-500 to-rose-400",
		"hotel neo":      "bg-gradient-to-br from-orange-600 to-yellow-500",
		"kamuela":        "bg-gradient-to-br from-amber-800 to-yellow-700",
		"quest":          "bg-gradient-to-br from-violet-700 to-purple-600",
		"harper":         "bg-gradient-to-br from-stone-700 to-amber-900",
		"huxley":         "bg-gradient-to-br from-slate-900 to-rose-900",
		"nordic":         "bg-gradient-to-br from-sky-800 to-indigo-700",
		"four corners":   "bg-gradient-to-br from-lime-700 to-green-600",
	}
	if s, ok := styles[strings.ToLower(brandName)]; ok {
		return s
	}
	return "bg-gradient-to-br from-gray-700 to-gray-600"
}
