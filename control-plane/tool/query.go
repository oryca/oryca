package tool

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	Limit    = 10
	Offset   = 0
	MaxLimit = 10000

	opRegex   = "$regex"
	opOptions = "$options"
)

// CommonIgnoredParams are the query params that never become filter fields
var CommonIgnoredParams = []string{"sort", "offset", "limit", "search"}

// SearchWildcard builds a case-insensitive MongoDB regex filter
func SearchWildcard(search, key string) bson.M {
	return bson.M{key: bson.M{opRegex: search, opOptions: "i"}}
}

// ParseLimit reads and validates the limit query param
func ParseLimit(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, errors.New("Query param 'limit' is invalid")
	}
	if n < 1 || n > MaxLimit {
		return 0, fmt.Errorf("Query param 'limit' must be between 1 and %d", MaxLimit)
	}
	return n, nil
}

// ParseOffset reads and validates the offset query param
func ParseOffset(v string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, errors.New("Query param 'offset' is invalid")
	}
	if n < 0 {
		return 0, errors.New("Query param 'offset' must be >= 0")
	}
	return n, nil
}

// IsStringInSlice reports whether s is in the slice
func IsStringInSlice(s string, slice []string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// EscapeRegexPattern escapes the regex metacharacters in a pattern
func EscapeRegexPattern(pattern string) string {
	specialChars := []string{"\\", ".", "+", "*", "?", "(", ")", "[", "]", "{", "}", "^", "$", "|"}
	for _, char := range specialChars {
		pattern = strings.ReplaceAll(pattern, char, "\\"+char)
	}
	return pattern
}

// OptionSortBson turns a sort query param such as "name:asc,createdAt:desc" into a bson.D
func OptionSortBson(value string) (bson.D, error) {
	order := bson.D{}

	if strings.TrimSpace(value) == "" {
		return order, errors.New("wrong format")
	}

	for _, qo := range strings.Split(value, ",") {
		qo, _ = url.QueryUnescape(qo)
		qo = strings.ReplaceAll(qo, "+", " ")
		parts := strings.Split(qo, ":")

		if len(parts) > 2 {
			return order, errors.New("wrong format")
		}

		key := strings.TrimSpace(parts[0])
		if key == "id" {
			key = "_id"
		}

		direction := 1
		if len(parts) == 2 {
			dir := strings.ToLower(strings.TrimSpace(parts[1]))
			if !IsStringInSlice(dir, []string{"asc", "desc"}) {
				return order, errors.New("wrong direction, should be asc or desc")
			}
			if dir == "desc" {
				direction = -1
			}
		}

		order = append(order, bson.E{Key: key, Value: direction})
	}

	return order, nil
}

// DatetimeInterval holds a range parsed from the OGC datetime format
type DatetimeInterval struct {
	Start *time.Time
	End   *time.Time
}

// ParseDatetimeInterval parse OGC API Features datetime value
// Accepted shapes: "instant", "start/end", "../end", "start/.."
func ParseDatetimeInterval(value string) (DatetimeInterval, error) {
	if !strings.Contains(value, "/") {
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return DatetimeInterval{}, fmt.Errorf("invalid datetime value: %s", value)
		}
		return DatetimeInterval{Start: &t, End: &t}, nil
	}
	parts := strings.SplitN(value, "/", 2)
	var interval DatetimeInterval
	if parts[0] != ".." && parts[0] != "" {
		t, err := time.Parse(time.RFC3339, parts[0])
		if err != nil {
			return DatetimeInterval{}, fmt.Errorf("invalid datetime start value: %s", parts[0])
		}
		interval.Start = &t
	}
	if parts[1] != ".." && parts[1] != "" {
		t, err := time.Parse(time.RFC3339, parts[1])
		if err != nil {
			return DatetimeInterval{}, fmt.Errorf("invalid datetime end value: %s", parts[1])
		}
		interval.End = &t
	}
	return interval, nil
}

// DatetimeIntervalFilter builds a MongoDB range filter for the given field and interval
func DatetimeIntervalFilter(field string, interval DatetimeInterval) bson.M {
	cond := bson.M{}
	if interval.Start != nil {
		cond["$gte"] = *interval.Start
	}
	if interval.End != nil {
		cond["$lte"] = *interval.End
	}
	if len(cond) == 0 {
		return bson.M{}
	}
	return bson.M{field: cond}
}

// ParseDatetimeParams reads OGC-style datetime query params for the allowed fields. It returns
// []bson.M ready to merge into an $and filter, the same shape GenerateFilterBson returns.
func ParseDatetimeParams(params url.Values, allowedFields []string) ([]bson.M, error) {
	var filters []bson.M
	for _, field := range allowedFields {
		v := params.Get(field)
		if v == "" {
			continue
		}
		interval, err := ParseDatetimeInterval(v)
		if err != nil {
			return nil, fmt.Errorf("Query param '%s' is invalid: %s", field, err.Error())
		}
		if f := DatetimeIntervalFilter(field, interval); len(f) > 0 {
			filters = append(filters, f)
		}
	}
	return filters, nil
}

// GenerateFilterBson turns URL query params into the []bson.M of an $and filter. It understands
// regex (*), null, exclude (!) and comma-separated multi-value OR.
// ignoredParams are the params to skip, such as sort, skip and limit.
func GenerateFilterBson(queryParams url.Values, ignoredParams []string) ([]bson.M, error) {
	and := []bson.M{}

	for key, values := range queryParams {
		if IsStringInSlice(key, ignoredParams) {
			continue
		}

		// map id to _id, and .id to ._id, to match the bson field names
		if key == "id" {
			key = "_id"
		} else {
			key = strings.ReplaceAll(key, ".id", "._id")
		}

		or := []bson.M{}
		for _, v := range values {
			valueSlices := strings.Split(v, ",")
			// with several values, also keep the raw one, in case the comma belongs to the value
			if len(valueSlices) > 1 {
				valueSlices = append(valueSlices, v)
			}

			for _, vs := range valueSlices {
				vs = strings.TrimSpace(vs)

				if len(vs) == 0 {
					return nil, fmt.Errorf("Query param '%s' has empty value", key)
				}

				if vs[0:1] == "!" {
					// Exclude mode
					vs = vs[1:]
					notConditions := []bson.M{}

					if strings.ToLower(vs) == "null" {
						or = append(or, bson.M{key: bson.M{"$exists": true, "$ne": nil}})
						continue
					}

					if string(vs[0]) == "*" && string(vs[len(vs)-1]) == "*" {
						vs = EscapeRegexPattern(strings.ReplaceAll(vs, "*", ""))
						notConditions = append(notConditions, bson.M{key: bson.M{"$not": bson.M{opRegex: vs, opOptions: "i"}}})
					} else if string(vs[0]) == "*" {
						vs = EscapeRegexPattern(strings.ReplaceAll(vs, "*", ""))
						notConditions = append(notConditions, bson.M{key: bson.M{"$not": bson.M{opRegex: vs + "$", opOptions: "i"}}})
					} else if vs[len(vs)-1:] == "*" {
						vs = EscapeRegexPattern(strings.ReplaceAll(vs, "*", ""))
						notConditions = append(notConditions, bson.M{key: bson.M{"$not": bson.M{opRegex: "^" + vs, opOptions: "i"}}})
					} else {
						escaped := EscapeRegexPattern(vs)
						notConditions = append(notConditions, bson.M{key: bson.M{"$not": bson.M{opRegex: "^" + escaped + "$", opOptions: "i"}}})

						if f, err := strconv.ParseFloat(vs, 64); err == nil {
							notConditions = append(notConditions, bson.M{key: bson.M{"$ne": f}})
						}
						if o, err := bson.ObjectIDFromHex(vs); err == nil {
							notConditions = append(notConditions, bson.M{key: bson.M{"$ne": o}})
						}
						if b, err := strconv.ParseBool(vs); err == nil {
							notConditions = append(notConditions, bson.M{key: bson.M{"$ne": b}})
						}
					}

					if len(notConditions) > 0 {
						or = append(or, bson.M{"$and": notConditions})
					}
				} else {
					// Include mode
					if strings.ToLower(vs) == "null" {
						or = append(or, bson.M{key: bson.M{"$in": bson.A{nil}}})
						or = append(or, bson.M{key: bson.M{"$exists": false}})
						continue
					}

					if string(vs[0]) == "*" && string(vs[len(vs)-1]) == "*" {
						vs = EscapeRegexPattern(strings.ReplaceAll(vs, "*", ""))
						or = append(or, bson.M{key: bson.M{opRegex: vs, opOptions: "i"}})
					} else if string(vs[0]) == "*" {
						vs = EscapeRegexPattern(strings.ReplaceAll(vs, "*", ""))
						or = append(or, bson.M{key: bson.M{opRegex: vs + "$", opOptions: "i"}})
					} else if vs[len(vs)-1:] == "*" {
						vs = EscapeRegexPattern(strings.ReplaceAll(vs, "*", ""))
						or = append(or, bson.M{key: bson.M{opRegex: "^" + vs, opOptions: "i"}})
					} else {
						escaped := EscapeRegexPattern(vs)
						or = append(or, bson.M{key: bson.M{opRegex: "^" + escaped + "$", opOptions: "i"}})

						if f, err := strconv.ParseFloat(vs, 64); err == nil {
							or = append(or, bson.M{key: f})
						}
						if o, err := bson.ObjectIDFromHex(vs); err == nil {
							or = append(or, bson.M{key: o})
						}
						if b, err := strconv.ParseBool(vs); err == nil {
							or = append(or, bson.M{key: b})
						}
					}
				}
			}
		}

		if len(or) > 0 {
			and = append(and, bson.M{"$or": or})
		}
	}

	return and, nil
}
