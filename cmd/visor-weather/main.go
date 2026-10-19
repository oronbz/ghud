// Command visor-weather is the Weather Plugin visor ships with: it speaks the
// plugin protocol on stdin and stdout, like any external Plugin.
package main

import (
	"os"

	"github.com/oronbz/visor/internal/weather"
)

func main() {
	os.Exit(weather.Main())
}
