package report

//
//import (
//	"context"
//
//	"github.com/brave-intl/bat-go/services/settlement/lib/payout"
//	"github.com/influxdata/influxdb/services/storage"
//)
//
//type Uploader interface {
//	Upload(ctx context.Context, config payout.Config) (*CompletedUpload, error)
//}
//
//type Notifier interface {
//	Notify(ctx context.Context, payoutID, reportURI string, versionID string) error
//}
//
//type Report struct {
//	txStore  storage.Store
//	uploader Uploader
//	notifier Notifier
//}
//
//func (r *Report) Upload() error {
//	r.uploader.Upload()
//}
