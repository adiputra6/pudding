package workers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jrallison/go-workers"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/travis-pro/worker-manager-service/common"
)

func init() {
	defaultQueueFuncs["instance-builds"] = instanceBuildsMain
}

func instanceBuildsMain(cfg *config, msg *workers.Msg) {
	buildPayloadJSON := []byte(msg.OriginalJson())
	buildPayload := &common.InstanceBuildPayload{}

	err := json.Unmarshal(buildPayloadJSON, buildPayload)
	if err != nil {
		log.WithField("err", err).Error("failed to deserialize message")
	}

	err = newInstanceBuilderWorker(buildPayload.InstanceBuild(),
		cfg.AWSAuth, cfg.AWSRegion).Build()
	if err != nil {
		log.WithField("err", err).Panic("instance build failed")
	}
}

type instanceBuilderWorker struct {
	ec2    *ec2.EC2
	sg     *ec2.SecurityGroup
	sgName string
	ami    *ec2.Image
	b      *common.InstanceBuild
	i      []*ec2.Instance
}

func newInstanceBuilderWorker(b *common.InstanceBuild, auth aws.Auth, region aws.Region) *instanceBuilderWorker {
	return &instanceBuilderWorker{
		b:      b,
		i:      []*ec2.Instance{},
		sgName: fmt.Sprintf("docker-worker-%d", time.Now().UTC().Unix()),
		ec2:    ec2.New(auth, region),
	}
}

func (ibw *instanceBuilderWorker) Build() error {
	var err error
	ibw.ami, err = common.ResolveAMI(ibw.ec2, ibw.b.AMI)
	if err != nil {
		log.WithFields(logrus.Fields{
			"ami_id": ibw.b.AMI,
			"err":    err,
		}).Error("failed to resolve ami")
		return err
	}

	err = ibw.createSecurityGroup()
	if err != nil {
		log.WithFields(logrus.Fields{
			"security_group_name": ibw.sgName,
			"err": err,
		}).Error("failed to create security group")
		return err
	}

	err = ibw.createInstances()
	if err != nil {
		log.WithField("err", err).Error("failed to create instance(s)")
		return err
	}

	err = ibw.tagInstances()
	if err != nil {
		log.WithField("err", err).Error("failed to tag instance(s)")
		return err
	}

	err = ibw.waitForInstances()
	if err != nil {
		log.WithField("err", err).Error("failed to wait for instance(s)")
		return err
	}

	ibw.notifyInstancesLaunched()
	err = ibw.setupInstances()
	if err != nil {
		log.WithField("err", err).Error("failed to set up instance(s)")
		return err
	}

	return nil
}

func (ibw *instanceBuilderWorker) createSecurityGroup() error {
	newSg := ec2.SecurityGroup{Name: ibw.sgName}
	resp, err := ibw.ec2.CreateSecurityGroup(newSg)
	if err != nil {
		log.WithField("err", err).Error("failed to create security group")
		return err
	}

	ibw.sg = &resp.SecurityGroup
	return nil
}

func (ibw *instanceBuilderWorker) createInstances() error {
	log.WithFields(logrus.Fields{
		"instance_type": ibw.b.InstanceType,
		"ami.id":        ibw.ami.Id,
		"ami.name":      ibw.ami.Name,
		"count":         ibw.b.Count,
	}).Info("booting instance")

	resp, err := ibw.ec2.RunInstances(&ec2.RunInstances{
		ImageId:        ibw.ami.Id,
		InstanceType:   ibw.b.InstanceType,
		SecurityGroups: []ec2.SecurityGroup{*ibw.sg},
	})
	if err != nil {
		return err
	}

	for _, inst := range resp.Instances {
		ibw.i = append(ibw.i, &inst)
	}

	return nil
}

func (ibw *instanceBuilderWorker) tagInstances() error {
	_, err := ibw.ec2.CreateTags(ibw.instanceIDs(), []ec2.Tag{
		ec2.Tag{Key: "role", Value: "worker"},
		ec2.Tag{Key: "Name", Value: fmt.Sprintf("travis-%s-%s-%s-%d", ibw.b.Site, ibw.b.Env, ibw.b.Queue, time.Now().UTC().Unix())},
		ec2.Tag{Key: "site", Value: ibw.b.Site},
		ec2.Tag{Key: "env", Value: ibw.b.Env},
		ec2.Tag{Key: "queue", Value: ibw.b.Queue},
	})

	return err
}

func (ibw *instanceBuilderWorker) waitForInstances() error {
	for {
		resp, err := ibw.ec2.DescribeInstanceStatus(&ec2.DescribeInstanceStatus{
			InstanceIds:         ibw.instanceIDs(),
			IncludeAllInstances: true,
			MaxResults:          int64(len(ibw.i)),
		}, &ec2.Filter{})

		if err != nil {
			return err
		}

		statuses := map[string]int{}

		for _, st := range resp.InstanceStatus {
			statuses[st.InstanceStatus.Status] = 1
		}

		if _, ok := statuses["pending"]; !ok {
			return nil
		}

		time.Sleep(5 * time.Second)
	}
}

func (ibw *instanceBuilderWorker) notifyInstancesLaunched() {
	// TODO: notify instance launched
	log.WithFields(logrus.Fields{
		"instance_ids": ibw.instanceIDs(),
	}).Info("launched instances")
}

func (ibw *instanceBuilderWorker) setupInstances() error {
	// TODO: setup instance
	return nil
}

func (ibw *instanceBuilderWorker) instanceIDs() []string {
	out := []string{}
	for _, inst := range ibw.i {
		out = append(out, inst.InstanceId)
	}
	return out
}
