// Command ghud-weather is the Weather Plugin ghud ships with: it speaks the
// plugin protocol on stdin and stdout, like any external Plugin.
package main

import (
	"os"

	"github.com/oronbz/ghud/internal/weather"
)

func main() {
	os.Exit(weather.Main())
}
