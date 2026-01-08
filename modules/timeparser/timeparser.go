package timeparser

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UniversalTime оборачивает time.Time и всегда хранит время в UTC
// Автоматически парсит различные форматы дат/времени и приводит к UTC
type UniversalTime struct {
	time.Time
}

// NewUniversalTime создает новый UniversalTime из time.Time, приводя к UTC
func NewUniversalTime(t time.Time) UniversalTime {
	return UniversalTime{Time: t.UTC()}
}

// NewUniversalTimeNow создает UniversalTime с текущим временем в UTC
func NewUniversalTimeNow() UniversalTime {
	return UniversalTime{Time: time.Now().UTC()}
}

// UnmarshalJSON реализует json.Unmarshaler
// Парсит строку в различных форматах и приводит к UTC
func (ut *UniversalTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	if s == "" || s == "null" {
		ut.Time = time.Time{}
		return nil
	}

	parsed, err := ParseUniversalTime(s)
	if err != nil {
		return err
	}

	ut.Time = parsed.Time
	return nil
}

// UnmarshalText реализует encoding.TextUnmarshaler
// Используется для парсинга из форм и других текстовых источников
func (ut *UniversalTime) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" {
		ut.Time = time.Time{}
		return nil
	}

	parsed, err := ParseUniversalTime(s)
	if err != nil {
		return err
	}

	ut.Time = parsed.Time
	return nil
}

// MarshalJSON реализует json.Marshaler
// Возвращает время в формате RFC3339 в UTC
func (ut UniversalTime) MarshalJSON() ([]byte, error) {
	if ut.Time.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(ut.Time.UTC().Format(time.RFC3339))
}

// MarshalText реализует encoding.TextMarshaler
// Используется для сериализации в текстовые форматы
func (ut UniversalTime) MarshalText() ([]byte, error) {
	if ut.Time.IsZero() {
		return []byte(""), nil
	}
	return []byte(ut.Time.UTC().Format(time.RFC3339)), nil
}

// String возвращает строковое представление в RFC3339
func (ut UniversalTime) String() string {
	if ut.Time.IsZero() {
		return ""
	}
	return ut.Time.UTC().Format(time.RFC3339)
}

// IsZero проверяет, является ли время нулевым
func (ut UniversalTime) IsZero() bool {
	return ut.Time.IsZero()
}

// ParseUniversalTime парсит строку в UniversalTime
// Поддерживает все форматы дат/времени и приводит к UTC
func ParseUniversalTime(s string) (UniversalTime, error) {
	if s == "" {
		return UniversalTime{}, nil
	}

	s = strings.TrimSpace(s)

	// Список форматов для парсинга
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		time.RFC1123,
		time.RFC1123Z,
		time.RFC822,
		time.RFC822Z,
		time.RFC850,
		time.ANSIC,
		time.UnixDate,
		time.RubyDate,
		time.Kitchen,
		time.Stamp,
		time.StampMilli,
		time.StampMicro,
		time.StampNano,
		"2006-01-02T15:04:05Z",           // ISO8601
		"2006-01-02T15:04:05.000Z",       // ISO8601 с миллисекундами
		"2006-01-02T15:04:05.000000Z",    // ISO8601 с микросекундами
		"2006-01-02T15:04:05.000000000Z", // ISO8601 с наносекундами
		"2006-01-02",                     // Date only
		"2006-01-02 15:04:05",            // Common format
		"2006/01/02 15:04:05",            // Common format
		"01/02/2006 15:04:05",            // US format
		"02.01.2006 15:04:05",            // European format
		"02-01-2006 15:04:05",            // European format
		"2006-01-02 15:04:05.000",        // With milliseconds
		"2006-01-02 15:04:05.000000",     // With microseconds
		"2006-01-02 15:04:05.000000000",  // With nanoseconds
	}

	// Пробуем каждый формат
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return UniversalTime{Time: t.UTC()}, nil
		}
	}

	// Пробуем парсить как Unix timestamp (секунды)
	if timestamp, err := parseUnixTimestamp(s); err == nil {
		return UniversalTime{Time: timestamp.UTC()}, nil
	}

	// Пробуем парсить как Unix timestamp (миллисекунды)
	if timestamp, err := parseUnixTimestampMillis(s); err == nil {
		return UniversalTime{Time: timestamp.UTC()}, nil
	}

	return UniversalTime{}, fmt.Errorf("unable to parse time: %s", s)
}

// parseUnixTimestamp парсит Unix timestamp в секундах
func parseUnixTimestamp(s string) (time.Time, error) {
	var sec int64
	if _, err := fmt.Sscanf(s, "%d", &sec); err != nil {
		return time.Time{}, err
	}

	// Проверяем, что это разумный timestamp (между 1970 и 2100 годом)
	if sec < 0 || sec > 4102444800 {
		return time.Time{}, fmt.Errorf("invalid unix timestamp: %d", sec)
	}

	return time.Unix(sec, 0), nil
}

// parseUnixTimestampMillis парсит Unix timestamp в миллисекундах
func parseUnixTimestampMillis(s string) (time.Time, error) {
	var millis int64
	if _, err := fmt.Sscanf(s, "%d", &millis); err != nil {
		return time.Time{}, err
	}

	// Проверяем, что это разумный timestamp (между 1970 и 2100 годом)
	if millis < 0 || millis > 4102444800000 {
		return time.Time{}, fmt.Errorf("invalid unix timestamp: %d", millis)
	}

	// Если число слишком большое для секунд, но разумное для миллисекунд
	if millis > 4102444800 {
		sec := millis / 1000
		nsec := (millis % 1000) * 1000000
		return time.Unix(sec, nsec), nil
	}

	return time.Time{}, fmt.Errorf("not a millisecond timestamp: %d", millis)
}
