package launcher

import "fyshos.com/fynedesk"

func init() {
	fynedesk.RegisterModule(calcMeta)
	fynedesk.RegisterModule(largeTypeMeta)
	fynedesk.RegisterModule(urlMeta)
	fynedesk.RegisterModule(unytsMeta)
}
