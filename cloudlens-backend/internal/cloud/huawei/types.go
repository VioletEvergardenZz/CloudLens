// 本文件用于定义华为云只读云资产模型。
// 文件职责：让华为云 ECS 与 CES 监控复用控制台已有的通用字段合同。
// 边界与容错：只表达查询结果，不包含任何会修改云资源的操作参数。

package huawei

import "github.com/VioletEvergardenZz/CloudLens/cloudlens-backend/internal/cloud/common"

type Config struct {
	AccessKeyID     string
	AccessKeySecret string
	ProjectID       string
	Region          string
	Regions         []string
	MetricPeriod    string
}

type Instance = common.Instance
type MetricPoint = common.MetricPoint
type MetricSeries = common.MetricSeries
