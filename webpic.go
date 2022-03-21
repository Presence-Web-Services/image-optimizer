package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	"github.com/jdeng/goheif"
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
	widthFlag := flag.String("widths", "288", "The pixel widths separated by commas that you want to generate")
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
	orient, err := getOrientation(file)
	if err != nil {
		// handle
		return
	}
	img, err := getImage(file)
	if err != nil {
		// handle
		return
	}
	fmt.Println(img.Bounds())
	img, err = transformAccordingly(img, orient)
	if err != nil {
		// handle
		return
	}
	fmt.Println(img.Bounds())
	img = resizeAccordingly(img, widths)
	fmt.Println(img.Bounds())
	extension := filepath.Ext(file)
	fileName := strings.TrimSuffix(file, extension)
	outFile, err := os.Create(fmt.Sprintf("%s.jpg", fileName))
	if err != nil {
		// handle
		return
	}
	defer outFile.Close()
	jpeg.Encode(outFile, img, &jpeg.Options{Quality: *qual})
}

func resizeAccordingly(img image.Image, widths []int) image.Image {
	return imaging.Resize(img, widths[0], 0, imaging.Lanczos)
}

func transformAccordingly(img image.Image, orient uint16) (image.Image, error) {
	switch orient {
	case 1:
		if imgNrgba, ok := img.(*image.NRGBA); ok {
			return imgNrgba, nil
		} else {
			return nil, errors.New("could not generate nrgba image")
		}
	case 2:
		// flip horizontally
		return imaging.FlipH(img), nil
	case 3:
		// rotate 180
		return imaging.Rotate180(img), nil
	case 4:
		// rotate 180
		// flip horizontally
		newImg := imaging.Rotate180(img)
		return imaging.FlipH(newImg), nil
	case 5:
		// rotate 270
		// flip horizontally
		newImg := imaging.Rotate270(img)
		return imaging.FlipH(newImg), nil
	case 6:
		// rotate 270
		return imaging.Rotate270(img), nil
	case 7:
		// rotate 90
		// flip horizontally
		newImg := imaging.Rotate90(img)
		return imaging.FlipH(newImg), nil
	case 8:
		// rotate 90
		return imaging.Rotate90(img), nil
	default:
		if imgNrgba, ok := img.(*image.NRGBA); ok {
			return imgNrgba, fmt.Errorf("cannot work with orientation %d for image", orient)
		} else {
			return nil, fmt.Errorf("cannot work with orientation %d for image", orient)
		}
	}
}

func getImage(file string) (image.Image, error) {
	extension := filepath.Ext(file)
	fileType := strings.ToLower(strings.Trim(extension, "."))
	switch fileType {
	case "jpg":
		fmt.Println("working with jpg")
		return getImageFromJpeg(file)
	case "png":
		fmt.Println("working with png")
		return getImageFromPng(file)
	case "heic":
		fmt.Println("working with heic")
		return getImageFromHeic(file)
	default:
		return nil, fmt.Errorf("I don't know how to handle %s files", fileType)
	}
}

func getImageFromPng(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jpeg.Decode(f)
}

func getImageFromJpeg(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return jpeg.Decode(f)
}

func getImageFromHeic(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return goheif.Decode(f)
}

func getOrientationExif(file string) ([]byte, error) {
	orient, err := getOrientation(file)
	if err != nil {
		return nil, err
	}
	return createExifData(orient)
}

func createExifData(orient uint16) ([]byte, error) {
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return nil, err
	}
	ti := exif.NewTagIndex()
	ib := exif.NewIfdBuilder(im, ti, exifcommon.IfdStandardIfdIdentity, exifcommon.EncodeDefaultByteOrder)
	err = ib.AddStandardWithName("Orientation", []uint16{orient})
	if err != nil {
		return nil, err
	}
	be := exif.NewIfdByteEncoder()
	return be.EncodeToExif(ib)
}

func getOrientation(file string) (uint16, error) {
	exifByteSlice, err := exif.SearchFileAndExtractExif(file)
	if err != nil {
		return 1, err
	}
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return 1, err
	}
	ti := exif.NewTagIndex()
	_, index, err := exif.Collect(im, ti, exifByteSlice)
	if err != nil {
		return 1, err
	}

	orient := uint16(1)

	err = index.RootIfd.EnumerateTagsRecursively(func(i *exif.Ifd, ite *exif.IfdTagEntry) error {
		if ite.TagName() == "Orientation" {
			val, err := ite.Value()
			if err != nil {
				return err
			}

			switch val := val.(type) {
			case []uint16:
				if len(val) != 1 {
					return fmt.Errorf("orientation captures multiple values %s", file)
				}
				orient = val[0]
			case uint16:
				orient = val
			default:
				return fmt.Errorf("orientation value is not of right type %s", file)
			}
		}
		return nil
	})
	if err != nil {
		return 1, err
	}
	return orient, nil
}
