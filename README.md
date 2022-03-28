# Image-Optimizer

Image optimizer takes in HEIC, JPG, or PNG image files, and resizes and optimizes the images for the web.
It outputs HTML code representing the `<picture>` element required for optimal responsive HTML images.

to build:
`go build webpic.go`

to run:
`./webpic.go [options] image-file [image-file...]`

example:
`./webpic.go -qual="50" -widths="288,330,400" image.jpg`