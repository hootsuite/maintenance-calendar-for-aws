# maintenance-calendar-for-aws
> generate AWS maintenance calendar

Generates an iCal calendar with information about scheduled maintenance events retrieved from AWS Personal Health Dashboard

Uploads calendar to S3

This allows us to subscribe and view scheduled events from within Google Calendar for example.

## Installation

OS X & Linux:

```sh
go build main.go
```

Set up AWS IAM policy with the following permissions. 

```
{
    "Version": "2012-10-17",
    "Statement": [
         {
            "Action": [
                "s3:ListAllMyBuckets"
            ],
            "Effect": "Allow",
            "Resource": [
                "arn:aws:s3:::*"
            ]
        },
        {
            "Action": [
                "s3:ListBucket",
                "s3:ListBucketMultipartUploads",
                "s3:ListBucketVersions",
                "s3:ListMultipartUploadParts",
                "s3:GetBucketAcl"
            ],
            "Effect": "Allow",
            "Resource": [
                "arn:aws:s3:::BUCKETNAME"
            ]
        },
        {
            "Sid": "Stmt1407264241000",
            "Effect": "Allow",
            "Action": [
                "s3:Get*",
                "s3:List*",
                "s3:Put*"
            ],
            "Resource": [
                "arn:aws:s3:::BUCKETNAME{/PREFIX}",
                "arn:aws:s3:::BUCKETNAME{/PREFIX}/*"
            ]
        },
        {
            "Effect": "Allow",
            "Action": [
                "health:Describe*",
                "ec2:Describe*",
                "rds:Describe*"
            ],
            "Resource": "*"
        }
    ]
}
```

Setup AWS credentials as needed via `aws configure` or through an instance role policy

## Usage example

```sh
./main --bucket-region=us-east-1 --bucket=bucketname --prefix=maintenancecalendar --filename=webcal.ics
```

This would generate a local `webcal.ics` file and attempt to upload to S3 with the URL `https://s3.amazonaws.com/bucket/maintenancecalendar/webcal.ics`

The ICS URL can then be used in your local calendar client or Google Calendar. Slack can be authorized to read from Google Calendar for useful chat notification.

## Development setup

```sh
go get
```

## Release History

* 0.0.1
    * initial release

## Meta

Ronald Bow – [@rmkbow](https://twitter.com/rmkbow) – ronald.bow@hootsuite.com - [https://github.com/rmkbow](https://github.com/rmkbow) 

Distributed under the Apache license. See ``LICENSE`` for more information.

[https://github.com/hootsuite/maintenance-calendar-for-aws](https://github.com/hootsuite/maintenance-calendar-for-aws)

## Contributing

1. Fork it (<https://github.com/yourname/yourproject/fork>)
2. Create your feature branch (`git checkout -b feature/fooBar`)
3. Commit your changes (`git commit -am 'Add some fooBar'`)
4. Push to the branch (`git push origin feature/fooBar`)
5. Create a new Pull Request


