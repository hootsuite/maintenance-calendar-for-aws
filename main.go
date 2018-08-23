package main

import (
	"fmt"
	"time"
	"strconv"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/health"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/rmkbow/ical-go"
	"net/url"
	"os"
	"strings"
	"flag"
)

var aws_session *session.Session
var health_connection *health.Health
var ec2_connection map[string]*ec2.EC2
var rds_connection map[string]*rds.RDS
var elasticache_connection map[string]*elasticache.ElastiCache

func error_check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	local := flag.Bool("local", false, "Do not Upload file to S3")
	bucket_region := flag.String("bucket-region", "", "Region of the S3 bucket")
	bucket := flag.String("bucket", "", "Name of the S3 bucket")
	prefix := flag.String("prefix", "/", "Prefix of the S3 path (optional)")
	filename := flag.String("filename", "", "Filename of the local and destination file")
	flag.Parse()
	exit_flag := 0

	if *local == false {
		if *bucket_region == "" {
			exit_flag = 2
			fmt.Println("--bucket-region REGION")
		}
		if *bucket == "" {
			exit_flag = 2
			fmt.Println("--bucket BUCKETNAME")
		}
	}
	if *filename == "" {
		exit_flag = 2
		fmt.Println("--filename FILENAME")
	}

	if exit_flag != 0 {
		os.Exit(exit_flag)
	}

	initialize()
	calendar := calendar(calendar_events(health_events()))
	save_calendar_to_file(*filename, calendar)
	if *local == false {
		upload_file(*bucket_region, *bucket, *prefix, *filename)
	}
}

func initialize() {
	aws_session, _ = session.NewSession()
	ec2_connection = make(map[string]*ec2.EC2)
	health_connection = health.New(aws_session, &aws.Config{Region: aws.String("us-east-1")})
	rds_connection = make(map[string]*rds.RDS)
	elasticache_connection = make(map[string]*elasticache.ElastiCache)
}

func initialize_ec2_connection(region string) {
	ec2_connection[region] = ec2.New(aws_session, &aws.Config{Region: aws.String(region)})
}

func initialize_rds_connection(region string) {
	rds_connection[region] = rds.New(aws_session, &aws.Config{Region: aws.String(region)})
}

func initialize_elasticache_connection(region string) {
	elasticache_connection[region] = elasticache.New(aws_session, &aws.Config{Region: aws.String(region)})
}

func health_events() []*health.Event {
	describe_event_filter := &health.EventFilter{
		EventTypeCategories: []*string{aws.String("scheduledChange")},
		EventStatusCodes: []*string{aws.String("open"), aws.String("upcoming")},
	}
	describe_event_params := &health.DescribeEventsInput{
		Filter: describe_event_filter,
	}
	describe_event_params.SetMaxResults(100)
	health_events,_ := health_connection.DescribeEvents(describe_event_params)
	return health_events.Events
}

func resource_ids(health_arn *string) []string {
	describe_affected_entities_params := &health.DescribeAffectedEntitiesInput{
		Filter: &health.EntityFilter{
			EventArns: []*string{health_arn},
		},
	}
	describe_affected_entities_params.SetMaxResults(100)
	var resource_ids []string
	affected_entities, _ := health_connection.DescribeAffectedEntities(describe_affected_entities_params)
	for _, entity := range affected_entities.Entities {
		resource_ids = append(resource_ids,*entity.EntityValue)
	}
	return resource_ids
}

func process_event(health_arn *string) ([]string, string, string, string, *time.Time, *time.Time) {

	resource_ids := resource_ids(health_arn)

	describe_event_params := &health.DescribeEventDetailsInput{
		EventArns: []*string{health_arn},
	}
	detailed_events, _ := health_connection.DescribeEventDetails(describe_event_params)
	var description string
	var event_type string
	var event_service string
	var event_start_time *time.Time
	var event_end_time *time.Time
	for _, set := range detailed_events.SuccessfulSet {
		description = *set.EventDescription.LatestDescription
		event_type = *set.Event.EventTypeCode
		event_service = *set.Event.Service
		event_start_time = set.Event.StartTime
		event_end_time = set.Event.EndTime
	}
	return resource_ids, description, event_type, event_service, event_start_time, event_end_time
}

func ec2_instance_name(instance_id string, region string) string {
	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []*string{
					aws.String(instance_id),
				},
			},
		},
	}

	var ec2din_response *ec2.DescribeInstancesOutput
	var err error
	if ec2_connection[region] == nil {
		initialize_ec2_connection(region)
	}
	ec2din_response, err = ec2_connection[region].DescribeInstances(params)
	error_check(err)

	var instance_name string
	for _, reservations := range ec2din_response.Reservations {
		for _, instance := range reservations.Instances {
			for _, tag := range instance.Tags {
				if *tag.Key == "Name" {
					instance_name = url.QueryEscape(*tag.Value)
				}
			}
		}
	}
	return instance_name
}


func rds_maintenance_window(name string, region string) string {
	var rds_maintenance_window string

	if rds_connection[region] == nil {
		initialize_rds_connection(region)
	}

	describe_rds_cluster_filter := &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(name),
	}

	db_clusters,_ := rds_connection[region].DescribeDBClusters(describe_rds_cluster_filter)
	for _,db_cluster := range db_clusters.DBClusters {
		rds_maintenance_window = *db_cluster.PreferredMaintenanceWindow
	}

	if rds_maintenance_window != "" {
		return rds_maintenance_window
	}

	describe_rds_instance_filter := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(name),
	}
	db_instances,_ := rds_connection[region].DescribeDBInstances(describe_rds_instance_filter)
	for _,db_instance := range db_instances.DBInstances {
		rds_maintenance_window = *db_instance.PreferredMaintenanceWindow
	}

	return rds_maintenance_window
}

func elasticache_maintenance_window(name string, region string) string {
	var elasticache_maintenance_window string

	if elasticache_connection[region] == nil {
		initialize_elasticache_connection(region)
	}

        elasticache_name_normalized := strings.Replace(name, "/", "_", -1)
	elasticache_name_split_underscore := strings.Split(elasticache_name_normalized, "_")
	elasticache_name_underscore_trimmed := elasticache_name_split_underscore[:len(elasticache_name_split_underscore) - 2]
	elasticache_name := strings.Join(elasticache_name_underscore_trimmed,"_")


	elasticache_number_split_dash := strings.Split(name, "-")
	elasticache_number_dash_trimmed := elasticache_number_split_dash[len(elasticache_number_split_dash) -1]

	elasticache_number, _ := strconv.Atoi(elasticache_number_dash_trimmed)

	describe_elasticache_replica_filter := &elasticache.DescribeReplicationGroupsInput{
		ReplicationGroupId: aws.String(elasticache_name),
	}

	replica_sets,_ := elasticache_connection[region].DescribeReplicationGroups(describe_elasticache_replica_filter)

	var elasticache_cluster_name string

	for _,replica_set := range replica_sets.ReplicationGroups {
		elasticache_cluster_name = *replica_set.MemberClusters[elasticache_number -1]
	}

	describe_elasticache_cluster_filter := &elasticache.DescribeCacheClustersInput{
		CacheClusterId: aws.String(elasticache_cluster_name),
	}

	clusters,_ := elasticache_connection[region].DescribeCacheClusters(describe_elasticache_cluster_filter)


	for _,cluster := range clusters.CacheClusters {
		elasticache_maintenance_window = *cluster.PreferredMaintenanceWindow
	}
	return elasticache_maintenance_window
}


func calendar(calendar_events []ical.CalendarEvent) ical.Calendar {
	calendar := ical.Calendar{calendar_events}
	return calendar
}

func save_calendar_to_file(filename string, calendar ical.Calendar) {
	f, _ := os.Create(filename)
	defer f.Close()
	fmt.Fprintf(f, calendar.ToICS())
}

func calendar_event(id string, summary string, description string, location string, start_time *time.Time, end_time *time.Time) ical.CalendarEvent {
	calendar_event := ical.CalendarEvent{
		Id: id,
		Summary: summary,
		Description: description,
		Location: location,
		URL: "https://phd.aws.amazon.com/phd/home?region=us-east-1#/dashboard/scheduled-changes",
		StartAt: start_time,
		EndAt: end_time,
	}
	return calendar_event
}

func calendar_events(health_events []*health.Event) []ical.CalendarEvent {
	var calendar_events []ical.CalendarEvent
	for _, health_event := range health_events {
		health_arn := health_event.Arn
		event_affected_resources, event_description, full_event_type, event_service, event_start_time, event_end_time := process_event(health_arn)

		event_type := full_event_type

		switch full_event_type {
		case "AWS_EC2_INSTANCE_REBOOT_MAINTENANCE_SCHEDULED":
			event_type = "REBOOT"
		case "AWS_EC2_INSTANCE_POWER_MAINTENANCE_SCHEDULED":
			event_type = "REBOOT"
		case "AWS_EC2_SYSTEM_REBOOT_MAINTENANCE_SCHEDULED":
			event_type = "REBOOT"
		case "AWS_EC2_INSTANCE_RETIREMENT_SCHEDULED":
			event_type = "RETIREMENT"
		case "AWS_EC2_INSTANCE_NETWORK_MAINTENANCE_SCHEDULED":
			event_type = "NET MAINT"
		case "AWS_RDS_MAINTENANCE_SCHEDULED":
			event_type = "MAINT SCHEDULED"
		}

		calendar_event_id := *health_arn
		calendar_event_description := event_description
		calendar_event_start_time := event_start_time
		calendar_event_end_time := event_end_time
		calendar_event_location := *health_event.Region

		for _, event_affected_resource := range event_affected_resources {
			var calendar_event_summary string
			switch event_service {
			case "EC2":
				calendar_event_summary = event_type + " " + ec2_instance_name(event_affected_resource, *health_event.Region) + " " + event_affected_resource
			case "RDS":
				calendar_event_summary = event_service + " " + event_affected_resource + " " + event_type
				calendar_event_start_time_rds, calendar_event_end_time_rds := maintenance_time(calendar_event_start_time, rds_maintenance_window(event_affected_resource, *health_event.Region))
				calendar_event_start_time = &calendar_event_start_time_rds
				calendar_event_end_time = &calendar_event_end_time_rds
			case "ELASTICACHE":
				calendar_event_summary = event_service + " " + event_affected_resource + " " + event_type
				calendar_event_start_time_elasticache, calendar_event_end_time_elasticache := maintenance_time(calendar_event_start_time, elasticache_maintenance_window(event_affected_resource, *health_event.Region))
				calendar_event_start_time = &calendar_event_start_time_elasticache
				calendar_event_end_time = &calendar_event_end_time_elasticache
			default:
				calendar_event_summary = event_service + " " + event_affected_resource + " " + event_type
			}
			calendar_event := calendar_event(calendar_event_id + "_" + event_affected_resource, calendar_event_summary, calendar_event_description, calendar_event_location, calendar_event_start_time, calendar_event_end_time)
			calendar_events = append(calendar_events, calendar_event)
		}
	}
	return calendar_events
}

func maintenance_time(event_start *time.Time, maintenance_window string) (time.Time, time.Time) {

	//parsing maintenance time
	//maintenance_window is in string format ddd:hh24:mi-ddd:hh24:mi
	maintenance_window_start_end := strings.Split(maintenance_window, "-")
	maintenance_window_start := strings.Split(maintenance_window_start_end[0], ":")
	maintenance_window_end := strings.Split(maintenance_window_start_end[1], ":")
	// maintenance_window_start[0] for weekday
	// maintenance_window_start[1] for hour
	// maintenance_window_start[2] for minute

	maintenance_window_start_time, maintenance_window_end_time := next_maintenance_window(event_start, maintenance_window_start, maintenance_window_end)

	return maintenance_window_start_time, maintenance_window_end_time
}

func next_maintenance_window(base_time *time.Time, maintenance_window_start []string, maintenance_window_end []string) (time.Time, time.Time) {
	maintenance_window_start_weekday := weekday_from_shortname(maintenance_window_start[0])
	maintenance_window_start_hour_int64,_ := strconv.ParseInt(maintenance_window_start[1], 10, 8)
	maintenance_window_start_hour := int(maintenance_window_start_hour_int64)
	maintenance_window_start_minute_int64,_ := strconv.ParseInt(maintenance_window_start[2], 10, 8)
	maintenance_window_start_minute := int(maintenance_window_start_minute_int64)

	maintenance_window_end_weekday := weekday_from_shortname(maintenance_window_end[0])
	maintenance_window_end_hour_int64,_ := strconv.ParseInt(maintenance_window_end[1], 10, 8)
	maintenance_window_end_hour := int(maintenance_window_end_hour_int64)
	maintenance_window_end_minute_int64,_ := strconv.ParseInt(maintenance_window_end[2], 10, 8)
	maintenance_window_end_minute := int(maintenance_window_end_minute_int64)

	days_ahead := maintenance_window_start_weekday - base_time.Weekday()
	if days_ahead < 0 {
		days_ahead += 7
	} else if days_ahead == 0 && maintenance_window_start_hour < base_time.Hour() && maintenance_window_start_minute < base_time.Minute(){
		days_ahead += 7
	}
	next_date := base_time.AddDate(0,0,int(days_ahead))
	next_maintenance_window_start := time.Date(next_date.Year(), next_date.Month(), next_date.Day(), maintenance_window_start_hour, maintenance_window_start_minute, 0, 0, next_date.Location())

	next_maintenance_window_end := next_maintenance_window_start
	if maintenance_window_end_weekday != maintenance_window_start_weekday {
		next_maintenance_window_end = next_maintenance_window_end.AddDate(0,0,int(1))
	}
	next_maintenance_window_end = time.Date(next_maintenance_window_end.Year(), next_maintenance_window_end.Month(), next_maintenance_window_end.Day(), maintenance_window_end_hour, maintenance_window_end_minute, 0, 0, next_date.Location())
	return next_maintenance_window_start, next_maintenance_window_end
}

func weekday_from_shortname(shortname string) time.Weekday {
	var weekday time.Weekday
	switch shortname {
	case "sun":
		weekday = time.Weekday(0)
	case "mon":
		weekday = time.Weekday(1)
	case "tue":
		weekday = time.Weekday(2)
	case "wed":
		weekday = time.Weekday(3)
	case "thu":
		weekday = time.Weekday(4)
	case "fri":
		weekday = time.Weekday(5)
	case "sat":
		weekday = time.Weekday(6)
	}
	return weekday
}


func upload_file(bucket_region string, bucket string, prefix string, filename string) {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	file, open_err := os.Open(filename)
	if open_err != nil {
		error_check(open_err)
	}
	defer file.Close()
	sess, _ := session.NewSession(&aws.Config{Region: aws.String(bucket_region)})
	uploader := s3manager.NewUploader(sess)

	_, upload_err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key: aws.String(prefix + filename),
		Body: file,
		ACL: aws.String("public-read"),
	})
	if upload_err != nil {
		error_check(upload_err)
	}

	cal_url := ""
	if bucket_region == "us-east-1" {
		cal_url = "https://s3.amazonaws.com/" + bucket + prefix + filename;
	} else {
		cal_url = "https://s3-" + bucket_region + ".amazonaws.com/" + bucket + prefix + filename;
	}

	fmt.Printf("Successfully uploaded to %q\n", cal_url)
}
