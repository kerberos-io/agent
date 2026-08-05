//go:build !moq

package cloud

import "github.com/kerberos-io/agent/machinery/src/models"

// StartLiveStreamMoQ is disabled in the standard Agent build.
func StartLiveStreamMoQ(_ *models.Configuration, _ *models.Communication, _ bool) {}
