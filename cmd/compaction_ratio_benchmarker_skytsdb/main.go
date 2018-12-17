package main

import (
	"flag"
	"github.com/colinmarc/hdfs"
	"strconv"
	"os"
	"io/ioutil"
	"path/filepath"
	"log"
)

// Program option vars:
var (
	nameNodeUrl string
	path        string
	step        string
	dataSize    int64
	dataPath    string
)

// step case choices:
var steps = []string{"init", "calc"}

// Parse args:
func init() {
	flag.StringVar(&nameNodeUrl, "urls", "localhost:9000", "HDFS namenode URL")
	flag.StringVar(&path, "path", "/", "HDFS's file or directory to statistic size")
	flag.StringVar(&step, "step", steps[0], "It consists of two steps.'init' step will save initial size, 'calc' step will calculate compaction ratio")
	flag.Int64Var(&dataSize, "dataSize", -1, "Size(byte) of data to be bulk loaded")
	flag.StringVar(&dataPath, "dataPath", "./data.txt", "Path of data to be imported for calculating dataSize if parameter 'dataSize' is not specified")

	flag.Parse()

	// params check
	if nameNodeUrl == "" {
		log.Fatal("invalid nameNodeUrl specifier")
	}
	if path == "" {
		log.Fatal("invalid path specifier")
	}
	if !(step == steps[0] || step == steps[1]) {
		log.Fatal("invalid step specifier")
	}
	if step == steps[1] {
		//get file size
		if dataSize == -1 && dataPath != "" {
			fi, e := os.Stat(dataPath)
			if e != nil {
				log.Fatal(e)
			}
			// get the size
			dataSize = fi.Size()
		}
		if dataSize == -1 {
			log.Fatal("One of the parameters dataSize and dataPath must be specified")
		}
	}
}

func main() {
	base, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		log.Fatal(err)
		return
	}
	client, err := hdfs.New(nameNodeUrl)
	if err == nil {
		contentSummary, error := client.GetContentSummary(path)
		if error == nil {
			totalSize := contentSummary.Size()
			//init step save size to file
			if step == steps[0] {
				er := CreateHiddenFile(filepath.Join(base, ".iniSize"))
				if er == nil {
					//write size to hidden file
					err := WriteFile(filepath.Join(base, ".iniSize"), strconv.FormatInt(totalSize, 10))
					if err == nil {
						log.Println("Write initial size success,iniSize:" + strconv.FormatInt(totalSize, 10))
					} else {
						log.Fatal(err)
					}
				} else {
					log.Fatal(er)
				}
			}
			//after bulk load data calc step will calculate compaction ratio
			if step == steps[1] {
				//read init size from file
				size, error := ReadFile(filepath.Join(base, ".iniSize"))
				if error == nil {
					sizeInt, err := strconv.ParseFloat(size, 64)
					if err == nil {
						ratio := (float64(totalSize) - sizeInt) / float64(dataSize)
						log.Println("ratio:" + strconv.FormatFloat(ratio, 'f', 10, 64))
					} else {
						log.Println("string convert to float error,size:" + size)
					}
				} else {
					log.Fatal("Read iniSize file error")
				}
			}
		} else {
			log.Fatal(error)
		}
	} else {
		log.Fatal(err)
	}
}

//create empty file
func CreateHiddenFile(file string) error {
	_, err := os.OpenFile(file, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	return nil
}

func WriteFile(path, str string) error {
	data := []byte(str)

	// write the whole body at once
	err := ioutil.WriteFile(path, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

//notice read big data is dangerous
func ReadFile(path string) (string, error) {
	// read the whole file at once
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}