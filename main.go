package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

type parameters struct {
	logGroup    string
	logStream   string
	fileName    string
	region      string
	endpointURL string
	logs        []string
}

func parseOption(args []string) (parameters, error) {
	params := parameters{}

	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.StringVar(&params.logGroup, "log-group", "", "The name of the log group where you want to put logs. It is required.")
	flags.StringVar(&params.logStream, "log-stream", "", "The name of the log stream where you want to put logs. If you do not use this parameters, it uploads logs to latest log stream.")
	flags.StringVar(&params.region, "region", "", "The name of the region. Override the region configured in config file.")
	flags.StringVar(&params.endpointURL, "endpoint-url", "", "The url of endpoint. Override default endpoint with the given URL.")
	flags.StringVar(&params.fileName, "logs-file", "", "The path of file that includes log events. See https://github.com/x-color/awsputlogs")
	flags.Usage = func() {
		fmt.Fprintf(os.Stdout, "awsputlogs is tool to upload JSON and string logs to the AWS CloudWatch Logs easily.\n\n")
		fmt.Fprintf(os.Stdout, "Usage: \n")
		flags.PrintDefaults()
	}

	flags.Parse(args[1:])

	if params.logGroup == "" {
		return parameters{}, errors.New("argument error: --log-group is required")
	}
	params.logs = flags.Args()

	return params, nil
}

func parseLogEvents(data []byte) ([]string, error) {
	logs := make([]interface{}, 0)
	if err := json.Unmarshal(data, &logs); err != nil {
		return nil, err
	}

	events := make([]string, len(logs))
	for i, event := range logs {
		// Convert the event to a string if it is JSON format
		if _, ok := event.(map[string]interface{}); ok {
			b, err := json.Marshal(event)
			if err != nil {
				return nil, err
			}
			events[i] = string(b)
			continue
		}

		events[i] = fmt.Sprint(event)
	}

	return events, nil
}

func getLogEventsFromFile(fileName string) ([]string, error) {
	f, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return parseLogEvents(data)
}

func loadConfig(params parameters) (aws.Config, error) {
	paramsFns := []func(*config.LoadOptions) error{}

	if params.endpointURL != "" {
		endpointResolver := aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           params.endpointURL,
				SigningRegion: params.region,
			}, nil
		})
		paramsFns = append(paramsFns, config.WithEndpointResolver(endpointResolver))
	}

	if params.region != "" {
		paramsFns = append(paramsFns, config.WithRegion(params.region))
	}

	return config.LoadDefaultConfig(context.Background(), paramsFns...)
}

func getLatestLogStream(client *cloudwatchlogs.Client, logGroup string) (string, error) {
	param := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroup),
		Descending:   aws.Bool(true),
		OrderBy:      types.OrderByLastEventTime,
	}
	res, err := client.DescribeLogStreams(context.Background(), param)
	if err != nil {
		return "", err
	}
	if len(res.LogStreams) == 0 {
		return "", fmt.Errorf("no log stream error: log streams are not found in %s. you have to create log stream before running this tool", logGroup)
	}
	return *res.LogStreams[0].LogStreamName, nil
}

func putLogEvents(client *cloudwatchlogs.Client, logGroup, logStream string, logEvents []string) error {
	in := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName:        aws.String(logGroup),
		LogStreamNamePrefix: aws.String(logStream),
	}
	out, err := client.DescribeLogStreams(context.Background(), in)
	if err != nil {
		return err
	}
	if len(out.LogStreams) == 0 {
		return fmt.Errorf("not log stream error: %s is not found in %s", logStream, logGroup)
	}

	param := &cloudwatchlogs.PutLogEventsInput{
		LogEvents:     make([]types.InputLogEvent, len(logEvents)),
		LogGroupName:  aws.String(logGroup),
		LogStreamName: aws.String(logStream),
		SequenceToken: out.LogStreams[0].UploadSequenceToken,
	}

	for i, event := range logEvents {
		param.LogEvents[i] = types.InputLogEvent{
			Message:   aws.String(event),
			Timestamp: aws.Int64(time.Now().UnixNano() / int64(time.Millisecond)),
		}
	}

	_, err = client.PutLogEvents(context.Background(), param)
	return err
}

func exec() error {
	params, err := parseOption(os.Args)
	if err != nil {
		return err
	}

	if params.fileName != "" {
		params.logs, err = getLogEventsFromFile(params.fileName)
		if err != nil {
			return err
		}
	}

	if len(params.logs) == 0 {
		return errors.New("no logs error: logs are required. you must set the log to args or use --events-file parameters")
	}

	cfg, err := loadConfig(params)
	if err != nil {
		return err
	}

	client := cloudwatchlogs.NewFromConfig(cfg)

	if params.logStream == "" {
		params.logStream, err = getLatestLogStream(client, params.logGroup)
		if err != nil {
			return err
		}
	}

	return putLogEvents(client, params.logGroup, params.logStream, params.logs)
}

func main() {
	if err := exec(); err != nil {
		fmt.Println(err)
	}
}
