package workers

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"text/template"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/feeds"
	"github.com/jrallison/go-workers"
	"github.com/mitchellh/goamz/ec2"
	"github.com/travis-ci/pudding/lib"
	"github.com/travis-ci/pudding/lib/db"
)

func init() {
	defaultQueueFuncs["instance-builds"] = instanceBuildsMain
}

func instanceBuildsMain(cfg *internalConfig, msg *workers.Msg) {
	buildPayloadJSON := []byte(msg.OriginalJson())
	buildPayload := &lib.InstanceBuildPayload{
		Args: []*lib.InstanceBuild{
			lib.NewInstanceBuild(),
		},
	}

	err := json.Unmarshal(buildPayloadJSON, buildPayload)
	if err != nil {
		log.WithField("err", err).Panic("failed to deserialize message")
	}

	err = newInstanceBuilderWorker(buildPayload.InstanceBuild(),
		cfg, msg.Jid(), workers.Config.Pool.Get()).Build()
	if err != nil {
		log.WithField("err", err).Panic("instance build failed")
	}
}

type instanceBuilderWorker struct {
	rc     redis.Conn
	n      []lib.Notifier
	jid    string
	cfg    *internalConfig
	ec2    *ec2.EC2
	sg     *ec2.SecurityGroup
	sgName string
	ami    *ec2.Image
	b      *lib.InstanceBuild
	i      *ec2.Instance
	t      *template.Template
}

func newInstanceBuilderWorker(b *lib.InstanceBuild, cfg *internalConfig, jid string, redisConn redis.Conn) *instanceBuilderWorker {
	notifier := lib.NewSlackNotifier(cfg.SlackTeam, cfg.SlackToken)

	ibw := &instanceBuilderWorker{
		rc:  redisConn,
		jid: jid,
		cfg: cfg,
		n:   []lib.Notifier{notifier},
		b:   b,
		ec2: ec2.New(cfg.AWSAuth, cfg.AWSRegion),
		t:   cfg.InitScriptTemplate,
	}

	ibw.sgName = fmt.Sprintf("pudding-%d-%p", time.Now().UTC().Unix(), ibw)
	return ibw
}

func (ibw *instanceBuilderWorker) Build() error {
	var err error

	f := ec2.NewFilter()
	if ibw.b.Role != "" {
		f.Add("tag:role", ibw.b.Role)
	}

	log.WithFields(logrus.Fields{
		"jid":    ibw.jid,
		"filter": f,
	}).Debug("resolving ami")

	ibw.ami, err = lib.ResolveAMI(ibw.ec2, ibw.b.AMI, f)
	if err != nil {
		log.WithFields(logrus.Fields{
			"jid":    ibw.jid,
			"ami_id": ibw.b.AMI,
			"err":    err,
		}).Error("failed to resolve ami")
		return err
	}

	log.WithField("jid", ibw.jid).Debug("creating security group")
	err = ibw.createSecurityGroup()
	if err != nil {
		log.WithFields(logrus.Fields{
			"jid": ibw.jid,
			"security_group_name": ibw.sgName,
			"err": err,
		}).Error("failed to create security group")
		return err
	}

	log.WithField("jid", ibw.jid).Debug("creating instance")
	err = ibw.createInstance()
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
			"jid": ibw.jid,
		}).Error("failed to create instance(s)")
		return err
	}

	ibw.b.InstanceID = ibw.i.InstanceId

	log.WithField("jid", ibw.jid).Debug("tagging instance")
	err = ibw.tagInstance()
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
			"jid": ibw.jid,
		}).Error("failed to tag instance(s)")
		return err
	}

	ibw.notifyInstanceLaunched()

	log.WithField("jid", ibw.jid).Debug("all done")
	return nil
}

func (ibw *instanceBuilderWorker) createSecurityGroup() error {
	newSg := ec2.SecurityGroup{
		Name:        ibw.sgName,
		Description: "custom security group",
	}

	log.WithFields(logrus.Fields{
		"jid": ibw.jid,
		"security_group_name": ibw.sgName,
	}).Debug("creating security group")

	resp, err := ibw.ec2.CreateSecurityGroup(newSg)
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
			"jid": ibw.jid,
		}).Error("failed to create security group")
		return err
	}

	ibw.sg = &resp.SecurityGroup

	log.WithFields(logrus.Fields{
		"jid": ibw.jid,
		"security_group_name": ibw.sgName,
	}).Debug("authorizing port 22 on security group")

	_, err = ibw.ec2.AuthorizeSecurityGroup(*ibw.sg, []ec2.IPPerm{
		ec2.IPPerm{
			Protocol:  "tcp",
			FromPort:  22,
			ToPort:    22,
			SourceIPs: []string{"0.0.0.0/0"},
		},
	})
	if err != nil {
		log.WithFields(logrus.Fields{
			"err": err,
			"jid": ibw.jid,
			"security_group_name": ibw.sgName,
		}).Error("failed to authorize port 22")
		return err
	}

	return nil
}

func (ibw *instanceBuilderWorker) createInstance() error {
	log.WithFields(logrus.Fields{
		"jid":           ibw.jid,
		"instance_type": ibw.b.InstanceType,
		"ami.id":        ibw.ami.Id,
		"ami.name":      ibw.ami.Name,
		"count":         ibw.b.Count,
	}).Info("booting instance")

	userData, err := ibw.buildUserData()
	if err != nil {
		return err
	}

	resp, err := ibw.ec2.RunInstances(&ec2.RunInstances{
		ImageId:        ibw.ami.Id,
		UserData:       userData,
		InstanceType:   ibw.b.InstanceType,
		SecurityGroups: []ec2.SecurityGroup{*ibw.sg},
	})
	if err != nil {
		return err
	}

	ibw.i = &resp.Instances[0]
	return nil
}

func (ibw *instanceBuilderWorker) tagInstance() error {
	nameTmpl, err := template.New(fmt.Sprintf("name-template-%s", ibw.jid)).Parse(ibw.b.NameTemplate)
	if err != nil {
		return err
	}

	var nameBuf bytes.Buffer
	err = nameTmpl.Execute(&nameBuf, ibw.b)
	if err != nil {
		return err
	}

	tags := []ec2.Tag{
		ec2.Tag{Key: "Name", Value: nameBuf.String()},
		ec2.Tag{Key: "role", Value: ibw.b.Role},
		ec2.Tag{Key: "site", Value: ibw.b.Site},
		ec2.Tag{Key: "env", Value: ibw.b.Env},
		ec2.Tag{Key: "queue", Value: ibw.b.Queue},
	}

	log.WithFields(logrus.Fields{
		"jid":  ibw.jid,
		"tags": tags,
	}).Debug("tagging instance")

	_, err = ibw.ec2.CreateTags([]string{ibw.i.InstanceId}, tags)

	return err
}

func (ibw *instanceBuilderWorker) buildUserData() ([]byte, error) {
	webURL, err := url.Parse(ibw.cfg.WebHost)
	if err != nil {
		return nil, err
	}

	tmpAuth := feeds.NewUUID().String()
	webURL.User = url.UserPassword("x", tmpAuth)

	webURL.Path = fmt.Sprintf("/init-scripts/%s", ibw.b.ID)
	initScriptURL := webURL.String()

	webURL.Path = fmt.Sprintf("/instance-builds/%s", ibw.b.ID)
	instanceBuildURL := webURL.String()

	buf := &bytes.Buffer{}
	gzw, err := gzip.NewWriterLevel(buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}

	tw := &bytes.Buffer{}
	w := io.MultiWriter(tw, gzw)

	yml, err := lib.BuildInstanceSpecificYML(ibw.b.Site, ibw.b.Env, ibw.cfg.InstanceYML, ibw.b.Queue, ibw.b.Count)
	if err != nil {
		return nil, err
	}

	ymlString, err := yml.String()
	if err != nil {
		return nil, err
	}

	err = ibw.t.Execute(w, &initScriptContext{
		Env:              ibw.b.Env,
		Site:             ibw.b.Site,
		Queue:            ibw.b.Queue,
		Role:             ibw.b.Role,
		AMI:              ibw.b.AMI,
		InstanceType:     ibw.b.InstanceType,
		InstanceRSA:      ibw.cfg.InstanceRSA,
		SlackChannel:     ibw.b.SlackChannel,
		PapertrailSite:   yml.PapertrailSite,
		InstanceYML:      ymlString,
		InstanceBuildID:  ibw.b.ID,
		InstanceBuildURL: instanceBuildURL,
	})
	if err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		"jid":    ibw.jid,
		"script": tw.String(),
	}).Debug("rendered init script")

	err = gzw.Close()
	if err != nil {
		return nil, err
	}

	initScriptB64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	err = ibw.rc.Send("MULTI")
	if err != nil {
		return nil, err
	}

	scriptKey := db.InitScriptRedisKey(ibw.b.ID)
	err = ibw.rc.Send("SETEX", scriptKey, 600, initScriptB64)
	if err != nil {
		ibw.rc.Send("DISCARD")
		return nil, err
	}

	authKey := db.AuthRedisKey(ibw.b.ID)
	err = ibw.rc.Send("SETEX", authKey, 600, tmpAuth)
	if err != nil {
		ibw.rc.Send("DISCARD")
		return nil, err
	}

	_, err = ibw.rc.Do("EXEC")
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf("#include %s\n", initScriptURL)), nil
}

func (ibw *instanceBuilderWorker) notifyInstanceLaunched() {
	for _, notifier := range ibw.n {
		notifier.Notify(ibw.b.SlackChannel,
			fmt.Sprintf("Started instance `%s` for instance build *%s*", ibw.i.InstanceId, ibw.b.ID))
	}
}
