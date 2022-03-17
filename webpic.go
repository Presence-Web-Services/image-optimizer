package main

import (
	"flag"
	"fmt"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/jdeng/goheif"
	// "github.com/rwcarlsen/goexif/exif"

	heicexif "github.com/dsoprea/go-heic-exif-extractor"
)

var pre *string
var dpr *int
var qual *int
var widths []int
var wg sync.WaitGroup

func main() {
	flag.Usage = usage
	pre = flag.String("pre", "/", "The HTML prefix to the image path")
	dpr = flag.Int("dpr", 3, "The DPR to start generating images with")
	qual = flag.Int("qual", 25, "The quality of JPEG to encode (worst 1 <-> 100 best)")
	widthFlag := flag.String("widths", "288", "The widths separated by commas that you want to generate")
	flag.Parse()

	widths = getWidths(widthFlag)
	files := flag.Args()

	if len(files) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	if *qual < 1 || *qual > 100 {
		fmt.Fprintf(os.Stderr, "Invalid quality setting: %d. Must be value 1-100.", *qual)
		flag.Usage()
		os.Exit(1)
	}

	fmt.Println("Prefix:", *pre)
	fmt.Println("DPR:", *dpr)
	fmt.Println("Qual:", *qual)
	for _, width := range widths {
		fmt.Println("Width:", width)
	}

	for _, file := range files {
		fmt.Println("Input File:", file)
		wg.Add(1)
		go generate(file)
	}

	wg.Wait()

}

func getWidths(widths *string) []int {
	widthStringSlice := strings.Split(*widths, ",")
	widthIntSlice := make([]int, len(widthStringSlice))
	for i, widthString := range widthStringSlice {
		parsedInt, err := strconv.Atoi(widthString)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid, non-integer width provided: %s, ignoring.", widthString)
			continue
		}
		widthIntSlice[i] = parsedInt
	}
	return widthIntSlice
}

func usage() {
	fmt.Printf("Usage: %s [OPTIONS] inputFile [inputFile ...]\n", os.Args[0])
	flag.PrintDefaults()
}

func generate(file string) {
	defer wg.Done()
	extension := filepath.Ext(file)
	fileType := strings.ToLower(strings.Trim(extension, "."))
	fileName := strings.TrimSuffix(file, extension)

	f, err := os.Open(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open file: %s, ignoring.", file)
		return
	}
	defer f.Close()

	switch fileType {
	case "jpg":
		fmt.Println("working with jpg")
	case "png":
		fmt.Println("working with png")
	case "heic":
		// exifData, err := goheif.ExtractExif(f)
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "Could not extract exif data from file: %s, ignoring.", file)
		// 	return
		// }
		// metaData, err := exif.Decode(bytes.NewReader(exifData))
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "Could not extract exif data from file: %s, ignoring.", file)
		// 	return
		// }
		// fmt.Println(metaData.Get(exif.Orientation))

		hemp := new(heicexif.HeicExifMediaParser)
		mc, err := hemp.Parse(f, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not extract exif data from file: %s, ignoring.", file)
			return
		}

		rootIfd, _, err := mc.Exif()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not extract exif data from file: %s, ignoring.", file)
			return
		}
		fmt.Println(rootIfd)

		img, err := goheif.Decode(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not decode file: %s, ignoring.", file)
			return
		}
		jpgFile := fmt.Sprintf("%s.jpg", fileName)
		jf, err := os.Create(jpgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not create jpg file: %s, ignoring.", jpgFile)
			return
		}
		defer jf.Close()

		// jw, err := newWriterExif(jf, exifData)
		// if err != nil {
		// 	fmt.Fprintf(os.Stderr, "Could not create exif writer for file: %s, ignoring.", jpgFile)
		// 	return
		// }
		// err = jpeg.Encode(jw, img, &jpeg.Options{Quality: *qual})
		err = jpeg.Encode(jf, img, &jpeg.Options{Quality: *qual})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not encode jpg file: %s, ignoring.", jpgFile)
			return
		}
	}
}

type writerSkipper struct {
	w           io.Writer
	bytesToSkip int
}

func (w *writerSkipper) Write(data []byte) (int, error) {
	if w.bytesToSkip <= 0 {
		return w.w.Write(data)
	}

	if dataLen := len(data); dataLen < w.bytesToSkip {
		w.bytesToSkip -= dataLen
		return dataLen, nil
	}

	if n, err := w.w.Write(data[w.bytesToSkip:]); err == nil {
		n += w.bytesToSkip
		w.bytesToSkip = 0
		return n, nil
	} else {
		return n, err
	}
}

func newWriterExif(w io.Writer, exif []byte) (io.Writer, error) {
	writer := &writerSkipper{w, 2}
	soi := []byte{0xff, 0xd8}
	if _, err := w.Write(soi); err != nil {
		return nil, err
	}

	if exif != nil {
		app1Marker := 0xe1
		markerlen := 2 + len(exif)
		marker := []byte{0xff, uint8(app1Marker), uint8(markerlen >> 8), uint8(markerlen & 0xff)}
		if _, err := w.Write(marker); err != nil {
			return nil, err
		}

		if _, err := w.Write(exif); err != nil {
			return nil, err
		}
	}

	return writer, nil
}
