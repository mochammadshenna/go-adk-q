package rate

// Sentec REST API client.
//
// Reserved for future use — zero hotels currently use Sentec (all use
// SimpleBooking with hotel_channel = 'SB').
//
// From the PHP RoomRate controller:
//   POST https://api.booking.sentec.io/sm/api/availability/search
//   Content-Type: application/json
//   Basic Auth: username:password from credential_booking_engines table
//   Body: {"property_id": N, "stay": {"check_in": "...", "check_out": "..."}, ...}
//   Response: {"data": {"rooms": [{"room_id": N, "rate_plans": [...]}]}}
//
// ponytail: not implemented — no hotels use Sentec today.
// Implement when a brand switches to Sentec.
