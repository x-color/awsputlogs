package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func Test_parseOption(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    parameters
		wantErr bool
	}{
		{
			name: "Set correct arguments",
			args: []string{
				"awsputlogs",
				"--log-group", "/test/group",
				"--log-stream", "test-stream",
				"--region", "us-east-1",
				"--endpoint-url", "http://localhost:4566/",
				"--logs-file", "logs.json",
			},
			want: parameters{
				endpointURL: "http://localhost:4566/",
				fileName:    "logs.json",
				logGroup:    "/test/group",
				logs:        []string{},
				logStream:   "test-stream",
				region:      "us-east-1",
			},
			wantErr: false,
		},
		{
			name: "Set logs to an argument, not using --logs-file",
			args: []string{
				"awsputlogs",
				"--log-group", "/test/group",
				"--log-stream", "test-stream",
				"--region", "us-east-1",
				"--endpoint-url", "http://localhost:4566/",
				"[INFO] Start Server",
				"[ERROR] Failed to Start Server",
			},
			want: parameters{
				endpointURL: "http://localhost:4566/",
				logGroup:    "/test/group",
				logs: []string{
					"[INFO] Start Server",
					"[ERROR] Failed to Start Server",
				},
				logStream: "test-stream",
				region:    "us-east-1",
			},
			wantErr: false,
		},
		{
			name: "Set only required args",
			args: []string{
				"awsputlogs",
				"--log-group", "/test/group",
			},
			want: parameters{
				logGroup: "/test/group",
				logs:     []string{},
			},
			wantErr: false,
		},
		{
			name: "Don't set required args",
			args: []string{
				"awsputlogs",
			},
			want:    parameters{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOption(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOption() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseOption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseLogEvents(t *testing.T) {
	type args struct {
		data []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{
			name: "Parse JSON logs",
			args: args{
				data: []byte(`[
					{
						"level": "info",
						"message": "[INFO] Start Server"
					},
					{
						"level": "error",
						"message": "[ERROR] Failed to Start Server"
					}
				]`),
			},
			want: []string{
				`{"level":"info","message":"[INFO] Start Server"}`,
				`{"level":"error","message":"[ERROR] Failed to Start Server"}`,
			},
			wantErr: false,
		},
		{
			name: "Parse string logs",
			args: args{
				data: []byte(`[
					"[INFO] Start Server",
					"[ERROR] Failed to Start Server"
				]`),
			},
			want: []string{
				"[INFO] Start Server",
				"[ERROR] Failed to Start Server",
			},
			wantErr: false,
		},
		{
			name: "Parse string logs that include double quarts",
			args: args{
				data: []byte(`[
					"\"[INFO] Start Server\"",
					"\"[WARN] Failed to Start Server. Restarting\"",
					"[ERROR] \"Failed to Start Server\""
				]`),
			},
			want: []string{
				`"[INFO] Start Server"`,
				`"[WARN] Failed to Start Server. Restarting"`,
				`[ERROR] "Failed to Start Server"`,
			},
			wantErr: false,
		},
		{
			name: "Parse no log",
			args: args{
				data: []byte("[]"),
			},
			want:    []string{},
			wantErr: false,
		},
		{
			name: "Parse invalid format 01",
			args: args{
				data: []byte(`
					"[INFO] Start Server",
					"[WARN] Failed to Start Server. Restarting",
					"[ERROR] Failed to Start Server"
				`),
			},
			wantErr: true,
		},
		{
			name: "Parse invalid format 02",
			args: args{
				data: []byte(`{
					"level": "INFO",
					"message": "Start Server",
				}`),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogEvents(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLogEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseLogEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func setUpClient(endpointURL, region string) (*cloudwatchlogs.Client, error) {
	cfg, err := loadConfig(parameters{
		endpointURL: endpointURL,
		region:      region,
	})
	if err != nil {
		return nil, err
	}
	return cloudwatchlogs.NewFromConfig(cfg), nil
}

func isEnabledEndpoint(cli *cloudwatchlogs.Client) bool {
	_, err := cli.DescribeLogGroups(context.Background(), &cloudwatchlogs.DescribeLogGroupsInput{})
	return err == nil
}

func setUpLogGroup(cli *cloudwatchlogs.Client) (string, error) {
	for i := 0; i < 10; i++ {
		logGroupName := fmt.Sprintf("log-group-%X", rand.Int())
		in := &cloudwatchlogs.CreateLogGroupInput{
			LogGroupName: aws.String(logGroupName),
		}
		_, err := cli.CreateLogGroup(context.Background(), in)
		if err != nil {
			if errors.Is(err, &types.ResourceAlreadyExistsException{}) {
				continue
			}
			return "", err
		}
		return logGroupName, err
	}

	return "", errors.New("error: can not create log group(log-group-<RANDOM STRING>). please try again")
}

func deleteLogGroup(cli *cloudwatchlogs.Client, logGroup string) error {
	in := &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroup),
	}
	_, err := cli.DeleteLogGroup(context.Background(), in)
	return err
}

func setUpLogStreams(cli *cloudwatchlogs.Client, logGroup string, n int) ([]string, error) {
	logStreams := make([]string, n)
	for i := range logStreams {
		logStreams[i] = fmt.Sprintf("log-stream-%d", i)
	}
	for _, name := range logStreams {
		in := &cloudwatchlogs.CreateLogStreamInput{
			LogGroupName:  aws.String(logGroup),
			LogStreamName: aws.String(name),
		}
		if _, err := cli.CreateLogStream(context.Background(), in); err != nil {
			return nil, err
		}
	}
	return logStreams, nil
}

func setUpLogGroupAndStreams(cli *cloudwatchlogs.Client, n int) (string, []string, error) {
	logGroup, err := setUpLogGroup(cli)
	if err != nil {
		return "", nil, err
	}

	logStreams, err := setUpLogStreams(cli, logGroup, n)
	if err != nil {
		return "", nil, err
	}

	return logGroup, logStreams, nil
}

func checkLogs(cli *cloudwatchlogs.Client, logGroup, logStream string, logs []string) (bool, error) {
	in := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroup),
	}
	if logStream != "" {
		in.LogStreamNames = []string{logStream}
	}

	out, err := cli.FilterLogEvents(context.Background(), in)
	if err != nil {
		return false, err
	}

	return len(logs) <= len(out.Events), nil
}

func Test_exec(t *testing.T) {
	localStackEndpointURL := "http://localhost:4566/"
	localStackRegion := "us-east-1"
	cli, err := setUpClient(localStackEndpointURL, localStackRegion)
	if err != nil {
		t.Fatalf("failed to set up: %v", err)
	}

	if !isEnabledEndpoint(cli) {
		t.Fatal("failed to set up: could not find the localstack's endpoint")
	}

	t.Run("Put string logs", func(t *testing.T) {
		logGroup, logStreams, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		logs := []string{
			"[INFO] Start Server",
			"[ERROR] Failed to Start Server",
		}
		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", logStreams[0],
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
		}
		os.Args = append(os.Args, logs...)

		if err := exec(); err != nil {
			t.Errorf("exec() error = %v, wantErr %v", err, false)
			return
		}

		ok, err := checkLogs(cli, logGroup, logStreams[0], logs)
		if err != nil {
			t.Errorf("failed to check result: %v", err)
			return
		}
		if !ok {
			t.Error("failed to put logs. could not find logs in CloudWatch Logs")
			return
		}
	})

	t.Run("Put JSON logs", func(t *testing.T) {
		logGroup, logStreams, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", logStreams[0],
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
			"--logs-file", "testdata/json-log-events.json",
		}

		if err := exec(); err != nil {
			t.Errorf("exec() error = %v, wantErr %v", err, false)
			return
		}

		ok, err := checkLogs(cli, logGroup, logStreams[0], []string{
			"{\"level\":\"info\",\"message\":\"Start Server\"}",
			"{\"level\":\"error\",\"message\":\"Failed to Start Server\"}",
		})
		if err != nil {
			t.Errorf("failed to check result: %v", err)
			return
		}
		if !ok {
			t.Error("failed to put logs. could not find logs in CloudWatch Logs")
			return
		}
	})

	t.Run("Put string and JSON logs", func(t *testing.T) {
		logGroup, logStreams, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", logStreams[0],
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
			"--logs-file", "testdata/string-and-json-log-events.json",
		}

		if err := exec(); err != nil {
			t.Errorf("exec() error = %v, wantErr %v", err, false)
			return
		}

		ok, err := checkLogs(cli, logGroup, logStreams[0], []string{
			"{\"level\":\"info\",\"message\":\"Start Server\"}",
			"[ERROR] Failed to Start Server",
		})
		if err != nil {
			t.Errorf("failed to check result: %v", err)
			return
		}
		if !ok {
			t.Error("failed to put logs. could not find logs in CloudWatch Logs")
			return
		}
	})

	t.Run("Put logs to unspecified log stream", func(t *testing.T) {
		logGroup, _, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		logs := []string{
			"[INFO] Start Server",
			"[ERROR] Failed to Start Server",
		}
		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
		}
		os.Args = append(os.Args, logs...)

		if err := exec(); err != nil {
			t.Errorf("exec() error = %v, wantErr %v", err, false)
			return
		}

		ok, err := checkLogs(cli, logGroup, "", logs)
		if err != nil {
			t.Errorf("failed to check result: %v", err)
			return
		}
		if !ok {
			t.Error("failed to put logs. could not find logs in CloudWatch Logs")
			return
		}
	})

	t.Run("Invalid log group", func(t *testing.T) {
		logs := []string{
			"[INFO] Start Server",
			"[ERROR] Failed to Start Server",
		}
		os.Args = []string{
			"awsputlogs",
			"--log-group", fmt.Sprintf("uncreated-log-group-%v", rand.Int()),
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
		}
		os.Args = append(os.Args, logs...)

		if err := exec(); err == nil {
			t.Errorf("exec() error = %v, wantErr %v", err, false)
			return
		}
	})

	t.Run("Invalid log stream", func(t *testing.T) {
		logGroup, _, err := setUpLogGroupAndStreams(cli, 1)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		logs := []string{
			"[INFO] Start Server",
			"[ERROR] Failed to Start Server",
		}
		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", fmt.Sprintf("uncreated-log-stream-%v", rand.Int()),
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
		}
		os.Args = append(os.Args, logs...)

		if err := exec(); err == nil {
			t.Errorf("exec() error = %v, wantErr %v", err, true)
			return
		}
	})

	t.Run("No log stream", func(t *testing.T) {
		logGroup, err := setUpLogGroup(cli)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		logs := []string{
			"[INFO] Start Server",
			"[ERROR] Failed to Start Server",
		}
		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
		}
		os.Args = append(os.Args, logs...)

		if err := exec(); err == nil {
			t.Errorf("exec() error = %v, wantErr %v", err, true)
			return
		}
	})

	t.Run("Invalid file path", func(t *testing.T) {
		logGroup, logStreams, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", logStreams[0],
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
			"--logs-file", "testdata/no-file.json",
		}

		if err := exec(); err == nil {
			t.Errorf("exec() error = %v, wantErr %v", err, true)
			return
		}
	})

	t.Run("Invalid file", func(t *testing.T) {
		logGroup, logStreams, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", logStreams[0],
			"--region", localStackRegion,
			"--endpoint-url", localStackEndpointURL,
			"--logs-file", "testdata/invalid-file.json",
		}

		if err := exec(); err == nil {
			t.Errorf("exec() error = %v, wantErr %v", err, true)
			return
		}
	})

	t.Run("Invalid region", func(t *testing.T) {
		// The localstack does not check if specified a region is valid.
		// It can not check this test case with the localstack.
		// So it always passes this test case.
	})

	t.Run("Invalid endpoint url", func(t *testing.T) {
		logGroup, logStreams, err := setUpLogGroupAndStreams(cli, 3)
		if err != nil {
			t.Errorf("failed to set up: %v", err)
			return
		}
		defer func() {
			if err := deleteLogGroup(cli, logGroup); err != nil {
				t.Errorf("failed to clean up: %v", err)
			}
		}()

		logs := []string{
			"[INFO] Start Server",
			"[ERROR] Failed to Start Server",
		}
		os.Args = []string{
			"awsputlogs",
			"--log-group", logGroup,
			"--log-stream", logStreams[0],
			"--region", localStackRegion,
			"--endpoint-url", "https://localhost",
		}
		os.Args = append(os.Args, logs...)

		if err := exec(); err == nil {
			t.Errorf("exec() error = %v, wantErr %v", err, true)
			return
		}
	})
}
