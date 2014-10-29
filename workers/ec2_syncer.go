package workers

import (
	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
	"github.com/travis-pro/worker-manager-service/common"
)

type ec2Syncer struct {
	cfg *config
	ec2 *ec2.EC2
	log *logrus.Logger
	i   common.InstanceFetcherStorer
}

func newEC2Syncer(cfg *config, log *logrus.Logger) (*ec2Syncer, error) {
	i, err := common.NewInstances(cfg.RedisURL.String(), log, cfg.InstanceStoreExpiry)
	if err != nil {
		return nil, err
	}

	return &ec2Syncer{
		cfg: cfg,
		log: log,
		i:   i,
		ec2: ec2.New(cfg.AWSAuth, cfg.AWSRegion),
	}, nil
}

func (es *ec2Syncer) Sync() error {
	es.log.Debug("ec2 syncer fetching worker instances")
	instances, err := common.GetWorkerInstances(es.ec2)
	if err != nil {
		return err
	}

	es.log.Debug("ec2 syncer storing instances")
	err = es.i.Store(instances)
	if err != nil {
		return err
	}

	return nil
}