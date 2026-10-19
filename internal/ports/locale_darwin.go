//go:build darwin && cgo

package ports

/*
#cgo LDFLAGS: -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>

// visor_hour_format is the current locale's hour pattern ("H", "h a"), which
// honours the 24-hour time toggle; 0 when it doesn't fit buf.
static int visor_hour_format(char *buf, int n) {
	CFLocaleRef loc = CFLocaleCopyCurrent();
	CFStringRef f = CFDateFormatterCreateDateFormatFromTemplate(NULL, CFSTR("j"), 0, loc);
	CFRelease(loc);
	if (f == NULL) return 0;
	Boolean ok = CFStringGetCString(f, buf, n, kCFStringEncodingUTF8);
	CFRelease(f);
	return ok;
}

static long visor_first_weekday(void) {
	CFCalendarRef cal = CFCalendarCopyCurrent();
	long d = (long)CFCalendarGetFirstWeekday(cal);
	CFRelease(cal);
	return d;
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

var weekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

func systemLocale() SystemLocale {
	var loc SystemLocale
	buf := make([]byte, 64)
	if C.visor_hour_format((*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf))) != 0 {
		// The pattern's hour letter: H and k count 0-23 or 1-24, h and K 1-12 or 0-11.
		switch f := C.GoString((*C.char)(unsafe.Pointer(&buf[0]))); {
		case strings.ContainsAny(f, "Hk"):
			loc.TimeFormat = "24h"
		case strings.ContainsAny(f, "hK"):
			loc.TimeFormat = "12h"
		}
	}
	if d := int(C.visor_first_weekday()); d >= 1 && d <= 7 {
		loc.WeekStart = weekdays[d-1]
	}
	return loc
}
