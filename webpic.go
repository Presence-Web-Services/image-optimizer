package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

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

	exifByteSlice, err := getOrientationExif(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not extract orientation data for file %s.", file)
	}

	switch fileType {
	case "jpg":
		fmt.Println("working with jpg")
	case "png":
		fmt.Println("working with png")
	case "heic":
		fmt.Println("working with heic")
		img, err := getImageFromHeic(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not get image data from HEIC file %s.", file)
		}
		writeJpgFile(fmt.Sprintf("%s.jpg", fileName), exifByteSlice, img)
	}
}

// func contains(s []string, e string) bool {
// 	for _, a := range s {
// 		if a == e {
// 			return true
// 		}
// 	}
// 	return false
// }

func getOrientationExif(file string) ([]byte, error) {
	orient, err := extractOrientation(file)
	if err != nil {
		return nil, err
	}

	fmt.Println(orient)

	return createExifData(orient)

	// fmt.Println("root:")
	// fmt.Println(index.RootIfd.String())
	// fmt.Println("ifds:")
	// for _, ifd := range index.Ifds {
	// 	fmt.Println(ifd.String())
	// }
	// fmt.Println("tree:")
	// for key, val := range index.Tree {
	// 	fmt.Printf("key:%d val:%s\n", key, val.String())
	// }
	// fmt.Println("lookup:")
	// for key, val := range index.Lookup {
	// 	fmt.Printf("key:%s val:%s\n", key, val.String())
	// }

	// tagIdsToKeep := make([]uint16, 0)
	// tagIdsToDelete := make([]uint16, 0)
	// tagNamesToFind := []string{"Orientation", "ExifTag", "ColorSpace", "PixelXDimension", "PixelYDimension"}
	// err = index.RootIfd.EnumerateTagsRecursively(func(i *exif.Ifd, ite *exif.IfdTagEntry) error {
	// 	if contains(tagNamesToFind, ite.TagName()) {
	// 		tagIdsToKeep = append(tagIdsToKeep, ite.TagId())
	// 	} else {
	// 		tagIdsToDelete = append(tagIdsToDelete, ite.TagId())
	// 	}
	// 	return nil
	// })
	// if err != nil {
	// 	return nil, err
	// }

	// fmt.Println("Keep:")
	// for _, val := range tagIdsToKeep {
	// 	fmt.Printf("%x ", val)
	// }
	// fmt.Println()
	// fmt.Println("Delete:")
	// for _, val := range tagIdsToDelete {
	// 	fmt.Printf("%x ", val)
	// }
	// fmt.Println()

	// tagsToKeep := make([]*exif.IfdTagEntry, 0)
	// tagNamesToFind := []string{"Orientation", "ExifTag", "ColorSpace", "PixelXDimension", "PixelYDimension"}
	// for _, ifd := range index.Tree {
	// 	for i := 0; i < len(tagNamesToFind); {
	// 		tagName := tagNamesToFind[i]
	// 		tagsFound, err := ifd.FindTagWithName(tagName)
	// 		if err != nil {
	// 			i++
	// 			continue
	// 		}
	// 		if tagsFound != nil {
	// 			tagsToKeep = append(tagsToKeep, tagsFound...)
	// 			tagNamesToFind = append(tagNamesToFind[:i], tagNamesToFind[i+1:]...)
	// 		} else {
	// 			i++
	// 		}
	// 	}
	// }

	// tagIds := make([]uint16, len(tagsToKeep))
	// for i, tag := range tagsToKeep {
	// 	tagIds[i] = tag.TagId()
	// }
	// fmt.Println(tagIds)

	//next := index.RootIfd.NextIfd().String()
	// fmt.Println(next)
	//children := index.RootIfd.Children()
	//ifds := []*exif.Ifd{index.RootIfd}
	//ifds = append(ifds, children...)

	// ib := exif.NewIfdBuilderFromExistingChain(index.RootIfd)

	// for _, tagIdToDelete := range tagIdsToDelete {
	// 	n, err := ib.DeleteAll(tagIdToDelete)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	fmt.Printf("%d deleted. TagID: %x\n", n, tagIdToDelete)
	// }

	// fmt.Println("ifd tree:")
	// ib.PrintIfdTree()
	// fmt.Println("tag tree:")
	// ib.PrintTagTree()

	// for ib != nil {
	// 	fmt.Println("IFD Tree")
	// 	ib.PrintIfdTree()
	// 	fmt.Println("Tag tree")
	// 	ib.PrintTagTree()
	// 	for _, tagIdToDelete := range tagIdsToDelete {
	// 		n, err := ib.DeleteAll(tagIdToDelete)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		fmt.Printf("%d deleted. TagID: %x\n", n, tagIdToDelete)
	// 	}
	// 	ib, err = ib.NextIb()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	// err = index.RootIfd.EnumerateTagsRecursively(func(i *exif.Ifd, ite *exif.IfdTagEntry) error {
	// 	fmt.Println("i:")
	// 	fmt.Println(i.String())
	// 	fmt.Println("ite:")
	// 	fmt.Println(ite.String())
	// 	return nil
	// })

	// ib := exif.NewIfdBuilder(im, ti, index.RootIfd.IfdIdentity(), index.RootIfd.ByteOrder())
	// err = ib.AddTagsFromExisting(index.RootIfd, tagIds, nil)
	// if err != nil {
	// 	return nil, err
	// }

	// be := exif.NewIfdByteEncoder()
	// return be.EncodeToExif(ib)
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

func extractOrientation(file string) (uint16, error) {
	exifByteSlice, err := exif.SearchFileAndExtractExif(file)
	if err != nil {
		return 0, err
	}
	im, err := exifcommon.NewIfdMappingWithStandard()
	if err != nil {
		return 0, err
	}
	ti := exif.NewTagIndex()
	_, index, err := exif.Collect(im, ti, exifByteSlice)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	return orient, nil
}

func getImageFromHeic(file string) (image.Image, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return goheif.Decode(f)
}

func writeJpgFile(file string, exifByteSlice []byte, image image.Image) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	w, err := newWriterExif(f, exifByteSlice)
	if err != nil {
		return err
	}
	return jpeg.Encode(w, image, &jpeg.Options{Quality: *qual})
}

// Skip Writer for exif writing
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

// type myIfdBuilder struct {
// 	exif.IfdBuilder
// }

// func (ib *myIfdBuilder) DeleteTags(tags []uint16) (int, error) {
// 	ib.
// }
