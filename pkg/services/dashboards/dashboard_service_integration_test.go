// +build integration

package dashboards

import (
	"testing"

	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/services/guardian"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/models"
)

func TestIntegratedDashboardService(t *testing.T) {
	sqlStore := sqlstore.InitTestDB(t)
	const testOrgID int64 = 1

	t.Run("Given saved folders and dashboards in organization A", func(t *testing.T) {
		origUpdateAlerting := UpdateAlerting
		t.Cleanup(func() {
			UpdateAlerting = origUpdateAlerting
		})
		UpdateAlerting = func(orgID int64, dashboard *models.Dashboard, user *models.SignedInUser) error {
			return nil
		}

		savedFolder := saveTestFolder(t, "Saved folder", testOrgID, sqlStore)
		savedDashInFolder := saveTestDashboard(t, "Saved dash in folder", testOrgID, savedFolder.Id, sqlStore)
		saveTestDashboard(t, "Other saved dash in folder", testOrgID, savedFolder.Id, sqlStore)

		assert.Equal(t, "Saved folder", savedFolder.Title)
		assert.Equal(t, "saved-folder", savedFolder.Slug)
		assert.NotEqual(t, 0, savedFolder.Id)
		assert.True(t, savedFolder.IsFolder)
		assert.Equal(t, int64(0), savedFolder.FolderId)
		assert.NotEmpty(t, savedFolder.Uid)

		assert.Equal(t, "Saved dash in folder", savedDashInFolder.Title)
		assert.Equal(t, "saved-dash-in-folder", savedDashInFolder.Slug)
		assert.NotEqual(t, 0, savedDashInFolder.Id)
		assert.False(t, savedDashInFolder.IsFolder)
		assert.Equal(t, savedFolder.Id, savedDashInFolder.FolderId)
		assert.NotEmpty(t, savedDashInFolder.Uid)
	})
}

type dashboardPermissionScenarioContext struct {
	dashboardGuardianMock *guardian.FakeDashboardGuardian
}

type dashboardPermissionScenarioFunc func(t *testing.T, sc *dashboardPermissionScenarioContext)

func dashboardPermissionScenario(t *testing.T, desc string, mock *guardian.FakeDashboardGuardian, fn dashboardPermissionScenarioFunc) {
	t.Helper()

	t.Run(desc, func(t *testing.T) {
		origNewDashboardGuardian := guardian.New
		guardian.MockDashboardGuardian(mock)

		sc := &dashboardPermissionScenarioContext{
			dashboardGuardianMock: mock,
		}

		defer func() {
			guardian.New = origNewDashboardGuardian
		}()

		fn(t, sc)
	})
}

func permissionScenario(t *testing.T, desc string, canSave bool, fn dashboardPermissionScenarioFunc) {
	t.Helper()

	mock := &guardian.FakeDashboardGuardian{
		CanSaveValue: canSave,
	}
	dashboardPermissionScenario(t, desc, mock, fn)
}

func callSaveWithResult(t *testing.T, cmd models.SaveDashboardCommand, sqlStore *sqlstore.SQLStore) *models.Dashboard {
	t.Helper()

	dto := toSaveDashboardDto(cmd)
	res, err := NewService(sqlStore, sqlStore).SaveDashboard(&dto, false)
	require.NoError(t, err)

	return res
}

func callSaveWithError(cmd models.SaveDashboardCommand, sqlStore *sqlstore.SQLStore) error {
	dto := toSaveDashboardDto(cmd)
	_, err := NewService(sqlStore, sqlStore).SaveDashboard(&dto, false)
	return err
}

func saveTestDashboard(t *testing.T, title string, orgID, folderID int64, sqlStore *sqlstore.SQLStore) *models.Dashboard {
	t.Helper()

	cmd := models.SaveDashboardCommand{
		OrgId:    orgID,
		FolderId: folderID,
		IsFolder: false,
		Dashboard: simplejson.NewFromAny(map[string]interface{}{
			"id":    nil,
			"title": title,
		}),
	}

	dto := SaveDashboardDTO{
		OrgId:     orgID,
		Dashboard: cmd.GetDashboardModel(),
		User: &models.SignedInUser{
			UserId:  1,
			OrgRole: models.ROLE_ADMIN,
		},
	}

	res, err := NewService(sqlStore, sqlStore).SaveDashboard(&dto, false)
	require.NoError(t, err)

	return res
}

func saveTestFolder(t *testing.T, title string, orgID int64, sqlStore *sqlstore.SQLStore) *models.Dashboard {
	t.Helper()
	cmd := models.SaveDashboardCommand{
		OrgId:    orgID,
		FolderId: 0,
		IsFolder: true,
		Dashboard: simplejson.NewFromAny(map[string]interface{}{
			"id":    nil,
			"title": title,
		}),
	}

	dto := SaveDashboardDTO{
		OrgId:     orgID,
		Dashboard: cmd.GetDashboardModel(),
		User: &models.SignedInUser{
			UserId:  1,
			OrgRole: models.ROLE_ADMIN,
		},
	}

	res, err := NewService(sqlStore, sqlStore).SaveDashboard(&dto, false)
	require.NoError(t, err)

	return res
}

func toSaveDashboardDto(cmd models.SaveDashboardCommand) SaveDashboardDTO {
	dash := (&cmd).GetDashboardModel()

	return SaveDashboardDTO{
		Dashboard: dash,
		Message:   cmd.Message,
		OrgId:     cmd.OrgId,
		User:      &models.SignedInUser{UserId: cmd.UserId},
		Overwrite: cmd.Overwrite,
	}
}
