package handler

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
)

// minGzipBytes. Body เล็กกว่านี้ไม่คุ้ม CPU ที่ใช้บีบ
const minGzipBytes = 1024

// ctxKeyUncompressedSize เก็บขนาดก่อนบีบไว้ใน echo context. Log ต้องใช้ค่านี้เสมอ
const ctxKeyUncompressedSize = "oryca:uncompressed-size"

// gzipPool reuse gzip.Writer ลด allocation ต่อ request
var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

// compressibleCT. บีบเฉพาะ text-based content type; image/tile/binary ข้าม (บีบซ้ำเปลือง CPU)
func compressibleCT(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	switch ct {
	case "application/json", "application/geo+json", "application/xml", "application/javascript":
		return true
	}
	return strings.HasSuffix(ct, "+json") || strings.HasSuffix(ct, "+xml")
}

// shouldGzipResponse ตัดสินว่าบีบ response นี้ไหม. เรียกหลังตั้ง response headers แล้ว ก่อน WriteHeader
// bodyLen = -1 เมื่อไม่รู้ขนาดล่วงหน้า (stream mode)
func shouldGzipResponse(req *http.Request, h http.Header, statusCode int, bodyLen int64) bool {
	if statusCode < 200 || statusCode >= 300 || statusCode == http.StatusNoContent || statusCode == http.StatusPartialContent {
		return false
	}
	if bodyLen >= 0 && bodyLen < minGzipBytes {
		return false
	}
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		return false
	}
	// upstream บีบมาแล้ว (ไม่ควรเกิดเพราะเรา strip Accept-Encoding แต่กันไว้)
	if h.Get("Content-Encoding") != "" {
		return false
	}
	return compressibleCT(h.Get("Content-Type"))
}

// prepareGzipHeaders ตั้ง header สำหรับ response ที่จะบีบ. ต้องเรียกก่อน WriteHeader
func prepareGzipHeaders(h http.Header) {
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	h.Add("Vary", "Accept-Encoding")
}

const contentTypeJSON = "application/json"

// transformableCT. Transform engine parse ได้เฉพาะ text (JSON/XML); binary เช่น PNG/protobuf ห้ามส่งเข้า
// ใช้เกณฑ์เดียวกับ compressibleCT. Text-based ทั้งคู่
func transformableCT(ct string) bool {
	return compressibleCT(ct)
}

// writeJSONMaybeGzip เขียน JSON body บีบเมื่อเข้าเงื่อนไข. เก็บขนาดก่อนบีบใน context
// ให้ Proxy ใช้กับ log แทน Response().Size ซึ่งเป็นค่าหลังบีบ
func writeJSONMaybeGzip(c echo.Context, bodyJSON []byte) error {
	hdr := c.Response().Header()
	hdr.Set("Content-Type", contentTypeJSON)
	if shouldGzipResponse(c.Request(), hdr, http.StatusOK, int64(len(bodyJSON))) {
		prepareGzipHeaders(hdr)
		c.Set(ctxKeyUncompressedSize, int64(len(bodyJSON)))
		c.Response().WriteHeader(http.StatusOK)
		writeGzip(c, bodyJSON)
		return nil
	}
	return c.Blob(http.StatusOK, contentTypeJSON, bodyJSON)
}

// writeGzip เขียน body แบบบีบ. คืนจำนวน bytes "ก่อนบีบ" (source of truth ของ log)
func writeGzip(c echo.Context, body []byte) int64 {
	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(c.Response().Writer)
	n, _ := gz.Write(body)
	_ = gz.Close()
	gzipPool.Put(gz)
	return int64(n)
}
