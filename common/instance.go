package common

type Instance struct {
	Name         string `json:"name" redis:"name"`
	InstanceID   string `json:"id" redis:"instance_id"`
	InstanceType string `json:"instance_type" redis:"instance_type"`
	ImageID      string `json:"image_id" redis:"image_id"`
	IP           string `json:"ip" redis:"ip"`
	PrivateIP    string `json:"private_ip" redis:"private_ip"`
	LaunchTime   string `json:"launch_time" redis:"launch_time"`
	Queue        string `json:"queue" redis:"queue"`
	Env          string `json:"env" redis:"env"`
	Site         string `json:"site" redis:"site"`
}