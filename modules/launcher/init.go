package launcher

import "fyshos.com/tyde"

func init() {
	tyde.RegisterModule(calcMeta)
	tyde.RegisterModule(largeTypeMeta)
	tyde.RegisterModule(searchMeta)
	tyde.RegisterModule(urlMeta)
	tyde.RegisterModule(unytsMeta)
}
