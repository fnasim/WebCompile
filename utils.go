package main

import(
	"io"
	"github.com/kjk/betterguid"
	"time"
	"os"
	"syscall"
	"os/signal"
)

func HandleOSInterrupt(cleanup func()) {
	sig := make(chan os.Signal, 1)
  	signal.Notify(sig, os.Interrupt, syscall.SIGTERM) // two signals: CTRL+C and term
  
  	go func() {
    	<-sig
    	cleanup()
    	os.Exit(0)
  	}()
}

func createTempPath() (string, error) {
	uuid := betterguid.New()
	date := time.Now().Format("2006-01-15")
	runPath := PathForCodeStorage + "/" + date + uuid

	err := os.Mkdir(runPath, 0777)
	if err != nil {
		return runPath, err
	}

	return runPath, nil
}

func cp(dst, src string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}