package rbac

import (
	"context"

	"github.com/grafana/grafana/pkg/api/routing"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/registry"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/grafana/grafana/pkg/services/sqlstore/migrator"
	"github.com/grafana/grafana/pkg/setting"
)

// RBACService is the service implementing role based access control.
type RBACService struct {
	Cfg           *setting.Cfg          `inject:""`
	RouteRegister routing.RouteRegister `inject:""`
	SQLStore      *sqlstore.SQLStore    `inject:""`
	log           log.Logger
}

func init() {
	registry.RegisterService(&RBACService{})
}

// Init initializes the AlertingService.
func (ac *RBACService) Init() error {
	ac.log = log.New("rbac")

	seeder := &seeder{
		Service: ac,
		log:     ac.log,
	}

	// TODO: Seed all orgs
	err := seeder.Seed(context.TODO(), 1)
	if err != nil {
		return err
	}

	return nil
}

func (ac *RBACService) IsDisabled() bool {
	_, exists := ac.Cfg.FeatureToggles["new_authz"]
	return !exists
}

func (ac *RBACService) AddMigration(mg *migrator.Migrator) {
	if ac.IsDisabled() {
		return
	}

	addRBACMigrations(mg)
}
