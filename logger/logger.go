package logger

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bushubdegefu/blue-proxy/configs"
	"github.com/madflojo/tasks"
)

func Logfile() (*os.File, error) {

	// Custom File Writer for logging
	file, err := os.OpenFile("blue-proxy.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
		return nil, err
	}
	return file, nil

}

func ScheduledTasks() *tasks.Scheduler {
	//  initalizing scheduler for regullarly running tasks
	scheduler := tasks.New()

	clearIntervalLogs, _ := strconv.Atoi(configs.AppConfig.GetOrDefault("CLEAR_LOGS_INTERVAL", "2"))

	// Add a task to move to Logs Directory Every Interval, Interval to Be Provided From Configuration File
	blue_proxy_log_file, _ := Logfile()

	if _, err := scheduler.Add(&tasks.Task{
		Interval: time.Duration(clearIntervalLogs) * time.Minute,
		TaskFunc: func() error {

			blue_proxy_log_file.Truncate(0)

			return nil
		},
	}); err != nil {
		fmt.Println(err)

	}

	return scheduler
}
