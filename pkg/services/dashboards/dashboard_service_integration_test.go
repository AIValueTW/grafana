// +build integration

package dashboards

import (
	"testing"

	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/services/guardian"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/bus"
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
		savedDashInGeneralFolder := saveTestDashboard(t, "Saved dashboard in general folder", testOrgID, 0, sqlStore)
		otherSavedFolder := saveTestFolder(t, "Other saved folder", testOrgID, sqlStore)

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

		// Basic validation tests

		t.Run("When saving a dashboard with non-existing id", func(t *testing.T) {
			cmd := models.SaveDashboardCommand{
				OrgId: testOrgID,
				Dashboard: simplejson.NewFromAny(map[string]interface{}{
					"id":    float64(123412321),
					"title": "Expect error",
				}),
			}

			err := callSaveWithError(cmd, sqlStore)
			assert.Equal(t, models.ErrDashboardNotFound, err)
		})

		// Given other organization

		t.Run("Given organization B", func(t *testing.T) {
			var otherOrgId int64 = 2

			t.Run("When creating a dashboard with same id as dashboard in organization A", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: otherOrgId,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"id":    savedDashInFolder.Id,
						"title": "Expect error",
					}),
					Overwrite: false,
				}

				err := callSaveWithError(cmd, sqlStore)
				assert.Equal(t, models.ErrDashboardNotFound, err)
			})

			permissionScenario(t, "Given user has permission to save", true, func(t *testing.T, sc *dashboardPermissionScenarioContext) {
				t.Run("When creating a dashboard with same uid as dashboard in organization A", func(t *testing.T) {
					var otherOrgId int64 = 2
					cmd := models.SaveDashboardCommand{
						OrgId: otherOrgId,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"uid":   savedDashInFolder.Uid,
							"title": "Dash with existing uid in other org",
						}),
						Overwrite: false,
					}

					res := callSaveWithResult(t, cmd, sqlStore)

					t.Run("It should create a new dashboard in organization B", func(t *testing.T) {
						require.NotNil(t, res)

						query := models.GetDashboardQuery{OrgId: otherOrgId, Uid: savedDashInFolder.Uid}

						err := bus.Dispatch(&query)
						require.NoError(t, err)
						assert.NotEqual(t, savedDashInFolder.Id, query.Result.Id)
						assert.Equal(t, res.Id, query.Result.Id)
						assert.Equal(t, otherOrgId, query.Result.OrgId)
						assert.Equal(t, savedDashInFolder.Uid, query.Result.Uid)
					})
				})
			})
		})

		// Given user has no permission to save

		permissionScenario(t, "Given user has no permission to save", false, func(t *testing.T, sc *dashboardPermissionScenarioContext) {
			t.Run("When creating a new dashboard in the General folder", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"title": "Dash",
					}),
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				assert.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, int64(0), sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When creating a new dashboard in other folder, it should create dashboard guardian for other folder with correct arguments and rsult in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"title": "Dash",
					}),
					FolderId:  otherSavedFolder.Id,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, otherSavedFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When creating a new dashboard by existing title in folder, it should create dashboard guardian for folder with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"title": savedDashInFolder.Title,
					}),
					FolderId:  savedFolder.Id,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, savedFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When creating a new dashboard by existing UID in folder, it should create dashboard guardian for folder with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"uid":   savedDashInFolder.Uid,
						"title": "New dash",
					}),
					FolderId:  savedFolder.Id,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, savedFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When updating a dashboard by existing id in the General folder, it should create dashboard guardian for dashboard with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"id":    savedDashInGeneralFolder.Id,
						"title": "Dash",
					}),
					FolderId:  savedDashInGeneralFolder.FolderId,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				assert.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, savedDashInGeneralFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When updating a dashboard by existing id in other folder, it should create dashboard guardian for dashboard with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"id":    savedDashInFolder.Id,
						"title": "Dash",
					}),
					FolderId:  savedDashInFolder.FolderId,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, savedDashInFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When moving a dashboard by existing ID to other folder from General folder, it should create dashboard guardian for other folder with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"id":    savedDashInGeneralFolder.Id,
						"title": "Dash",
					}),
					FolderId:  otherSavedFolder.Id,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, otherSavedFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When moving a dashboard by existing id to the General folder from other folder, it should create dashboard guardian for General folder with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"id":    savedDashInFolder.Id,
						"title": "Dash",
					}),
					FolderId:  0,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				assert.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, int64(0), sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When moving a dashboard by existing uid to other folder from General folder, it should create dashboard guardian for other folder with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"uid":   savedDashInGeneralFolder.Uid,
						"title": "Dash",
					}),
					FolderId:  otherSavedFolder.Id,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, otherSavedFolder.Id, sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})

			t.Run("When moving a dashboard by existing UID to the General folder from other folder, it should create dashboard guardian for General folder with correct arguments and result in access denied error", func(t *testing.T) {
				cmd := models.SaveDashboardCommand{
					OrgId: testOrgID,
					Dashboard: simplejson.NewFromAny(map[string]interface{}{
						"uid":   savedDashInFolder.Uid,
						"title": "Dash",
					}),
					FolderId:  0,
					UserId:    10000,
					Overwrite: true,
				}

				err := callSaveWithError(cmd, sqlStore)
				require.Equal(t, models.ErrDashboardUpdateAccessDenied, err)

				assert.Equal(t, int64(0), sc.dashboardGuardianMock.DashId)
				assert.Equal(t, cmd.OrgId, sc.dashboardGuardianMock.OrgId)
				assert.Equal(t, cmd.UserId, sc.dashboardGuardianMock.User.UserId)
			})
		})

		// Given user has permission to save

		permissionScenario(t, "Given user has permission to save", true, func(t *testing.T, sc *dashboardPermissionScenarioContext) {
			t.Run("and overwrite flag is set to false", func(t *testing.T) {
				shouldOverwrite := false

				t.Run("When creating a dashboard in General folder with same name as dashboard in other folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInFolder.Title,
						}),
						FolderId:  0,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, res.Id, query.Result.Id)
					assert.Equal(t, int64(0), query.Result.FolderId)
				})

				t.Run("When creating a dashboard in other folder with same name as dashboard in General folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInGeneralFolder.Title,
						}),
						FolderId:  savedFolder.Id,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					assert.NotEqual(t, savedDashInGeneralFolder.Id, res.Id)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, savedFolder.Id, query.Result.FolderId)
				})

				t.Run("When creating a folder with same name as dashboard in other folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInFolder.Title,
						}),
						IsFolder:  true,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					assert.NotEqual(t, savedDashInGeneralFolder.Id, res.Id)
					assert.True(t, res.IsFolder)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, 0, query.Result.FolderId)
					assert.True(t, query.Result.IsFolder)
				})

				t.Run("When saving a dashboard without id and uid and unique title in folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"title": "Dash without id and uid",
						}),
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					assert.Greater(t, res.Id, 0)
					assert.NotEmpty(t, res.Uid)
					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, res.Id, query.Result.Id)
					assert.Equal(t, res.Uid, query.Result.Uid)
				})

				t.Run("When saving a dashboard when dashboard id is zero ", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    0,
							"title": "Dash with zero id",
						}),
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, res.Id, query.Result.Id)
				})

				t.Run("When saving a dashboard in non-existing folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"title": "Expect error",
						}),
						FolderId:  123412321,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardFolderNotFound, err)
				})

				t.Run("When updating an existing dashboard by id without current version", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    savedDashInGeneralFolder.Id,
							"title": "test dash 23",
						}),
						FolderId:  savedFolder.Id,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardVersionMismatch, err)
				})

				t.Run("When updating an existing dashboard by id with current version", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":      savedDashInGeneralFolder.Id,
							"title":   "Updated title",
							"version": savedDashInGeneralFolder.Version,
						}),
						FolderId:  savedFolder.Id,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: savedDashInGeneralFolder.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, "Updated title", query.Result.Title)
					assert.Equal(t, savedFolder.Id, query.Result.FolderId)
					assert.Greater(t, query.Result.Version, savedDashInGeneralFolder.Version)
				})

				t.Run("When updating an existing dashboard by uid without current version", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"uid":   savedDashInFolder.Uid,
							"title": "test dash 23",
						}),
						FolderId:  0,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardVersionMismatch, err)
				})

				t.Run("When updating an existing dashboard by uid with current version", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"uid":     savedDashInFolder.Uid,
							"title":   "Updated title",
							"version": savedDashInFolder.Version,
						}),
						FolderId:  0,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: savedDashInFolder.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, "Updated title", query.Result.Title)
					assert.Equal(t, int64(0), query.Result.FolderId)
					assert.Greater(t, query.Result.Version, savedDashInFolder.Version)
				})

				t.Run("When creating a dashboard with same name as dashboard in other folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInFolder.Title,
						}),
						FolderId:  savedDashInFolder.FolderId,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardWithSameNameInFolderExists, err)
				})

				t.Run("When creating a dashboard with same name as dashboard in General folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInGeneralFolder.Title,
						}),
						FolderId:  savedDashInGeneralFolder.FolderId,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardWithSameNameInFolderExists, err)
				})

				t.Run("When creating a folder with same name as existing folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedFolder.Title,
						}),
						IsFolder:  true,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardWithSameNameInFolderExists, err)
				})
			})

			t.Run("and overwrite flag is set to true", func(t *testing.T) {
				shouldOverwrite := true

				t.Run("When updating an existing dashboard by id without current version", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    savedDashInGeneralFolder.Id,
							"title": "Updated title",
						}),
						FolderId:  savedFolder.Id,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: savedDashInGeneralFolder.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, "Updated title", query.Result.Title)
					assert.Equal(t, savedFolder.Id, query.Result.FolderId)
					assert.Greater(t, query.Result.Version, savedDashInGeneralFolder.Version)
				})

				t.Run("When updating an existing dashboard by uid without current version", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"uid":   savedDashInFolder.Uid,
							"title": "Updated title",
						}),
						FolderId:  0,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: savedDashInFolder.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, "Updated title", query.Result.Title)
					assert.Equal(t, int64(0), query.Result.FolderId)
					assert.Greater(t, query.Result.Version, savedDashInFolder.Version)
				})

				t.Run("When updating uid for existing dashboard using id", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    savedDashInFolder.Id,
							"uid":   "new-uid",
							"title": savedDashInFolder.Title,
						}),
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.Nil(t, res)
					assert.Equal(t, savedDashInFolder.Id, res.Id)
					assert.Equal(t, "new-uid", res.Uid)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: savedDashInFolder.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, "new-uid", query.Result.Uid)
					assert.Greater(t, query.Result.Version, savedDashInFolder.Version)
				})

				t.Run("When updating uid to an existing uid for existing dashboard using id", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    savedDashInFolder.Id,
							"uid":   savedDashInGeneralFolder.Uid,
							"title": savedDashInFolder.Title,
						}),
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardWithSameUIDExists, err)
				})

				t.Run("When creating a dashboard with same name as dashboard in other folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInFolder.Title,
						}),
						FolderId:  savedDashInFolder.FolderId,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.NotNil(t, res)
					assert.Equal(t, savedDashInFolder.Id, res.Id)
					assert.Equal(t, savedDashInFolder.Uid, res.Uid)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, res.Id, query.Result.Id)
					assert.Equal(t, res.Uid, query.Result.Uid)
				})

				t.Run("When creating a dashboard with same name as dashboard in General folder", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: testOrgID,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    nil,
							"title": savedDashInGeneralFolder.Title,
						}),
						FolderId:  savedDashInGeneralFolder.FolderId,
						Overwrite: shouldOverwrite,
					}

					res := callSaveWithResult(t, cmd, sqlStore)
					require.Nil(t, res)
					assert.Equal(t, savedDashInGeneralFolder.Id, res.Id)
					assert.Equal(t, savedDashInGeneralFolder.Uid, res.Uid)

					query := models.GetDashboardQuery{OrgId: cmd.OrgId, Id: res.Id}

					err := bus.Dispatch(&query)
					require.NoError(t, err)
					assert.Equal(t, res.Id, query.Result.Id)
					assert.Equal(t, res.Uid, query.Result.Uid)
				})

				t.Run("When updating existing folder to a dashboard using id", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    savedFolder.Id,
							"title": "new title",
						}),
						IsFolder:  false,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardTypeMismatch, err)
				})

				t.Run("When updating existing dashboard to a folder using id", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"id":    savedDashInFolder.Id,
							"title": "new folder title",
						}),
						IsFolder:  true,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardTypeMismatch, err)
				})

				t.Run("When updating existing folder to a dashboard using uid", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"uid":   savedFolder.Uid,
							"title": "new title",
						}),
						IsFolder:  false,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardTypeMismatch, err)
				})

				t.Run("When updating existing dashboard to a folder using uid", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"uid":   savedDashInFolder.Uid,
							"title": "new folder title",
						}),
						IsFolder:  true,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardTypeMismatch, err)
				})

				t.Run("When updating existing folder to a dashboard using title", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"title": savedFolder.Title,
						}),
						IsFolder:  false,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardWithSameNameAsFolder, err)
				})

				t.Run("When updating existing dashboard to a folder using title", func(t *testing.T) {
					cmd := models.SaveDashboardCommand{
						OrgId: 1,
						Dashboard: simplejson.NewFromAny(map[string]interface{}{
							"title": savedDashInGeneralFolder.Title,
						}),
						IsFolder:  true,
						Overwrite: shouldOverwrite,
					}

					err := callSaveWithError(cmd, sqlStore)
					assert.Equal(t, models.ErrDashboardFolderWithSameNameAsDashboard, err)
				})
			})
		})
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
