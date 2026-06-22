# How to add support for a new hotel brand

Archipelago Hotels brand data lives in the central database (`db_archipelagowebsite`). The server discovers brands dynamically from `tb_brands` on startup, so most of the work is verifying the database records and then wiring up two code-side maps in `internal/repository/repository.go`.

## Step 1: Verify the brand exists in the central database

Connect to the central DB and confirm the brand row:

```sql
SELECT brand_id, brand_name, db_prefix_name, parent_brand_id, brand_color
FROM tb_brands
WHERE brand_name LIKE '%NewBrand%';
```

The `db_prefix_name` column is the key: it drives per-brand database discovery. If it is NULL or empty the server cannot connect to the brand DB and will skip live rates for all its hotels.

## Step 2: Add to `brandDBName` if the database name is non-standard

By default, the server constructs the per-brand database name as:

```
db_<db_prefix_name>website
```

For example, `db_prefix_name = "aston"` maps to `db_astonwebsite`.

Some brands do not follow this pattern. Check the actual database name on the MySQL server:

```sql
SHOW DATABASES LIKE 'db_%';
```

If the brand's database name does not match the pattern, add an entry to `brandDBName` in `internal/repository/repository.go`:

```go
var brandDBName = map[string]string{
    "favehotel": "db_favewebsite",
    "pba":       "db_pba",
    "newbrand":  "db_newbrand_prod",  // add here
}
```

The key is the lowercase `db_prefix_name` value from `tb_brands`.

## Step 3: Add a `brandImageStyle` entry

The dashboard renders a Tailwind gradient card for each hotel when no image is available. Add an entry for the new brand in the `brandImageStyle` function in `internal/repository/repository.go`:

```go
func brandImageStyle(brandName string) string {
    styles := map[string]string{
        // ... existing entries ...
        "new brand": "bg-gradient-to-br from-teal-700 to-cyan-600",
    }
    ...
}
```

The key is the lowercase `brand_name` from `tb_brands`. If no entry matches, the card falls back to a gray gradient — the search results still work, it just looks generic.

## Step 4: Test with search_hotels

Rebuild and restart, then exercise the brand filter:

```
search_hotels { "brand": "New Brand" }
```

Or in HTTP mode:

```sh
curl 'http://localhost:9011/api/hotels?brand=New+Brand'
```

You should see hotels whose `brand_name` matches. An empty result means either the brand name in the query does not match `tb_brands.brand_name`, or the central DB query returned no hotels for that brand.

## Step 5: Verify thumbnails load

The server checks for the `thumbnail_desktop` column in `tb_hotels` using `HasColumn` before attempting to load it. This check runs automatically when the brand DB first connects (via `scanColumns`, which inspects `INFORMATION_SCHEMA.COLUMNS` for `tb_hotels`, `tb_hrooms`, and `tb_hroom`).

To verify:

1. Enable debug logging: `DEBUG=1`
2. Run a search for the brand — you will see a log line: `brand DB connected prefix=newbrand db=db_newbrandwebsite`
3. If thumbnails are missing in the dashboard despite images existing in the DB, check that `tb_hotels.thumbnail_desktop` exists in the brand database:

```sql
SELECT COLUMN_NAME
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'db_newbrandwebsite'
  AND TABLE_NAME = 'tb_hotels'
  AND COLUMN_NAME = 'thumbnail_desktop';
```

If the column is absent, the server silently skips thumbnail loading and uses the gradient fallback — this is expected behaviour for older brand databases.
