package launcher

import "fyshos.com/fynedesk"

func init() {
	fynedesk.RegisterModule(calcMeta)
	fynedesk.RegisterModule(largeTypeMeta)
	fynedesk.RegisterModule(searchMeta)
	fynedesk.RegisterModule(urlMeta)
	fynedesk.RegisterModule(unytsMeta)
}
