// bulk_load_opentsdb loads an OpenTSDB daemon with data from stdin.
//
// The caller is responsible for assuring that the database is empty before
// bulk load.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/influxdata/influxdb-comparisons/util/report"
	//"github.com/klauspost/compress/gzip"
	"github.com/pkg/profile"
)

// Program option vars:
var (
	csvDaemonUrls  string
	daemonUrls     []string
	workers        int
	batchSize      int
	backoff        time.Duration
	doLoad         bool
	memprofile     bool
	reportDatabase string
	reportHost     string
	reportUser     string
	reportPassword string
	reportTagsCSV  string
)

// Global vars
var (
	bufPool        sync.Pool
	batchChan      chan *bytes.Buffer
	inputDone      chan struct{}
	workersGroup   sync.WaitGroup
	backingOffChan chan bool
	backingOffDone chan struct{}
	reportTags     [][2]string
	reportHostname string
)

// Parse args:
func init() {
	flag.StringVar(&csvDaemonUrls, "urls", "http://localhost:8080", "SkyTSDB URLs, comma-separated. Will be used in a round-robin fashion.")
	flag.IntVar(&batchSize, "batch-size", 5000, "Batch size (input lines).")
	flag.IntVar(&workers, "workers", 1, "Number of parallel requests to make.")
	//flag.DurationVar(&backoff, "backoff", time.Second, "Time to sleep between requests when server indicates backpressure is needed.")
	flag.BoolVar(&doLoad, "do-load", true, "Whether to write data. Set this flag to false to check input read speed.")
	flag.BoolVar(&memprofile, "memprofile", false, "Whether to write a memprofile (file automatically determined).")
	flag.StringVar(&reportDatabase, "report-database", "database_benchmarks", "Database name where to store result metrics")
	flag.StringVar(&reportHost, "report-host", "", "Host to send result metrics")
	flag.StringVar(&reportUser, "report-user", "", "User for host to send result metrics")
	flag.StringVar(&reportPassword, "report-password", "", "User password for Host to send result metrics")
	flag.StringVar(&reportTagsCSV, "report-tags", "", "Comma separated k:v tags to send  alongside result metrics")
	flag.Parse()

	daemonUrls = strings.Split(csvDaemonUrls, ",")
	if len(daemonUrls) == 0 {
		log.Fatal("missing 'urls' flag")
	}
	fmt.Printf("daemon URLs: %v\n", daemonUrls)

	if reportHost != "" {
		fmt.Printf("results report destination: %v\n", reportHost)
		fmt.Printf("results report database: %v\n", reportDatabase)

		var err error
		reportHostname, err = os.Hostname()
		if err != nil {
			log.Fatalf("os.Hostname() error: %s", err.Error())
		}
		fmt.Printf("hostname for results report: %v\n", reportHostname)

		if reportTagsCSV != "" {
			pairs := strings.Split(reportTagsCSV, ",")
			for _, pair := range pairs {
				fields := strings.SplitN(pair, ":", 2)
				tagpair := [2]string{fields[0], fields[1]}
				reportTags = append(reportTags, tagpair)
			}
		}
		fmt.Printf("results report tags: %v\n", reportTags)
	}
}

func main() {
	if memprofile {
		p := profile.Start(profile.MemProfile)
		defer p.Stop()
	}
	if doLoad {
		// check that there are no pre-existing databases:
		existingDatabases, err := listDatabases(daemonUrls[0])
		if err != nil {
			log.Fatal(err)
		}

		if len(existingDatabases) > 0 {
			log.Fatalf("There are databases already in the data store. If you know what you are doing, run the command:\ncurl 'http://localhost:8086/query?q=drop%%20database%%20%s'\n", existingDatabases[0])
		}
	}

	bufPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 4*1024*1024))
		},
	}

	batchChan = make(chan *bytes.Buffer, workers)
	inputDone = make(chan struct{})

	backingOffChan = make(chan bool, 100)
	backingOffDone = make(chan struct{})

	for i := 0; i < workers; i++ {
		daemonUrl := daemonUrls[i%len(daemonUrls)]
		workersGroup.Add(1)
		cfg := HTTPWriterConfig{
			Host: daemonUrl,
		}
		go processBatches(NewHTTPWriter(cfg))
	}

	go processBackoffMessages()

	start := time.Now()
	itemsRead := scan(batchSize)

	<-inputDone
	close(batchChan)

	workersGroup.Wait()

	close(backingOffChan)
	<-backingOffDone

	end := time.Now()
	took := end.Sub(start)
	rate := float64(itemsRead) / float64(took.Seconds())

	fmt.Printf("loaded %d items in %fsec with %d workers (mean values rate %f/sec)\n", itemsRead, took.Seconds(), workers, rate)

	if reportHost != "" {
		reportParams := &report.LoadReportParams{
			ReportParams: report.ReportParams{
				DBType:             "OpenTSDB",
				ReportDatabaseName: reportDatabase,
				ReportHost:         reportHost,
				ReportUser:         reportUser,
				ReportPassword:     reportPassword,
				ReportTags:         reportTags,
				Hostname:           reportHostname,
				DestinationUrl:     daemonUrls[0],
				Workers:            workers,
				ItemLimit:          -1,
			},
			IsGzip:    true,
			BatchSize: batchSize,
		}
		err := report.ReportLoadResult(reportParams, itemsRead, rate, -1, took)

		if err != nil {
			log.Fatal(err)
		}
	}
}

// scan reads one line at a time from stdin.
// When the requested number of lines per batch is met, send a batch over batchChan for the workers to write.
func scan(linesPerBatch int) int64 {
	buf := bufPool.Get().(*bytes.Buffer)
	//zw := gzip.NewWriter(buf)

	var n int
	var itemsRead int64

	openbracket := []byte("[")
	closebracket := []byte("]")
	commaspace := []byte(", ")
	//newline := []byte("\n")

	buf.Write(openbracket)
	//zw.Write(openbracket)
	//zw.Write(newline)

	scanner := bufio.NewScanner(bufio.NewReaderSize(os.Stdin, 4*1024*1024))
	for scanner.Scan() {
		itemsRead++
		if n > 0 {
			//写入逗号分隔符
			buf.Write(commaspace)
			//zw.Write(commaspace)
			//zw.Write(newline)
		}
		buf.Write(scanner.Bytes())
		//zw.Write(scanner.Bytes())

		n++
		if n >= linesPerBatch {
			//zw.Write(newline)
			buf.Write(closebracket)
			//zw.Write(closebracket)
			//zw.Close()

			batchChan <- buf

			buf = bufPool.Get().(*bytes.Buffer)
			//zw = gzip.NewWriter(buf)
			buf.Write(openbracket)
			//zw.Write(openbracket)
			//zw.Write(newline)
			n = 0
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %s", err.Error())
	}

	// Finished reading input, make sure last batch goes out.
	if n > 0 {
		//zw.Write(newline)
		buf.Write(closebracket)
		//zw.Write(closebracket)
		//zw.Close()
		batchChan <- buf
	}

	// Closing inputDone signals to the application that we've read everything and can now shut down.
	close(inputDone)

	return itemsRead
}

// processBatches reads byte buffers from batchChan and writes them to the target server, while tracking stats on the write.
func processBatches(w LineProtocolWriter) {
	for batch := range batchChan {
		// Write the batch: try until backoff is not needed.
		if doLoad {
			var err error
			for {
				_, err = w.WriteLineProtocol(batch.Bytes())
				if err == BackoffError {
					backingOffChan <- true
					time.Sleep(backoff)
				} else {
					backingOffChan <- false
					break
				}
			}
			if err != nil {
				log.Fatalf("Error writing: %s\n", err.Error())
			}
		}
		//fmt.Println(string(batch.Bytes()))

		// Return the batch buffer to the pool.
		batch.Reset()
		bufPool.Put(batch)
	}
	workersGroup.Done()
}

func processBackoffMessages() {
	var totalBackoffSecs float64
	var start time.Time
	last := false
	for this := range backingOffChan {
		if this && !last {
			start = time.Now()
			last = true
		} else if !this && last {
			took := time.Now().Sub(start)
			fmt.Printf("backoff took %.02fsec\n", took.Seconds())
			totalBackoffSecs += took.Seconds()
			last = false
			start = time.Now()
		}
	}
	fmt.Printf("backoffs took a total of %fsec of runtime\n", totalBackoffSecs)
	backingOffDone <- struct{}{}
}

// TODO(rw): listDatabases lists the existing data in OpenTSDB.
func listDatabases(daemonUrl string) ([]string, error) {
	return nil, nil
}
