package coach

import _ "embed"

//go:embed web/index.html
var coachIndexHTML string

//go:embed web/coach.css
var coachCSS string

//go:embed web/coach.js
var coachJS string

//go:embed web/audio-worklet.js
var audioWorkletJS string
