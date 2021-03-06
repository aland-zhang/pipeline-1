// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package authadapter

import (
	"context"
	"errors"
	"io/ioutil"
	"regexp"
	"testing"

	"github.com/jinzhu/gorm"

	//  SQLite driver used for integration test
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/banzaicloud/pipeline/src/auth"
)

func setUpDatabase(t *testing.T) *gorm.DB {
	db, err := gorm.Open("sqlite3", "file::memory:")
	require.NoError(t, err)

	logger := logrus.New()
	logger.SetOutput(ioutil.Discard)

	err = auth.Migrate(db, logger)
	require.NoError(t, err)

	return db
}

func TestGormOrganizationStore_EnsureOrganizationExists(t *testing.T) {
	// This causes `concurrent map write` issues during tests
	// t.Parallel()

	t.Run("create", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		created, id, err := store.EnsureOrganizationExists(context.Background(), "example", "github")
		require.NoError(t, err)

		var organization auth.Organization

		err = db.
			Where(auth.Organization{Name: "example"}).
			First(&organization).
			Error
		require.NoError(t, err)

		assert.True(t, created)
		assert.Equal(t, organization.ID, id)
		assert.Equal(t, organization.Name, "example")
		assert.Equal(t, organization.NormalizedName, "example")
	})

	t.Run("already_exists", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		organization := auth.Organization{Name: "example", Provider: "github"}

		err := db.Save(&organization).Error
		require.NoError(t, err)

		created, id, err := store.EnsureOrganizationExists(context.Background(), "example", "github")
		require.NoError(t, err)

		assert.False(t, created)
		assert.Equal(t, organization.ID, id)
	})

	t.Run("conflict", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		organization := auth.Organization{Name: "example", Provider: "github"}

		err := db.Save(&organization).Error
		require.NoError(t, err)

		created, id, err := store.EnsureOrganizationExists(context.Background(), "example", "gitlab")
		require.Error(t, err)

		assert.True(t, errors.Is(err, auth.ErrOrganizationConflict))
		assert.False(t, created)
		assert.Equal(t, uint(0), id)
	})

	t.Run("same_normalized_name", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		const name1 = "john.doe@dev.example.com"
		created1, id1, err := store.EnsureOrganizationExists(context.Background(), name1, "github")
		require.NoError(t, err)

		const name2 = "john.doe@dev-example.com"
		created2, id2, err := store.EnsureOrganizationExists(context.Background(), name2, "github")
		require.NoError(t, err)

		var organization1 auth.Organization

		err = db.
			Where(auth.Organization{Name: name1}).
			First(&organization1).
			Error
		require.NoError(t, err)

		assert.True(t, created1)
		assert.Equal(t, organization1.ID, id1)
		assert.Equal(t, organization1.Name, name1)
		assert.Equal(t, organization1.NormalizedName, "john-doe-dev-example-com")

		var organization2 auth.Organization

		err = db.
			Where(auth.Organization{Name: name2}).
			First(&organization2).
			Error
		require.NoError(t, err)

		assert.True(t, created2)
		assert.Equal(t, organization2.ID, id2)
		assert.Equal(t, organization2.Name, name2)
		assert.Regexp(t, regexp.MustCompile("john-doe-dev-example-com-[a-zA-Z]{6}"), organization2.NormalizedName)
	})
}

func TestGormOrganizationStore_GetOrganizationMembershipsOf(t *testing.T) {
	db := setUpDatabase(t)
	store := NewGormOrganizationStore(db)

	user := auth.User{
		Name:  "John Doe",
		Email: "john.doe@example.com",
		Login: "john.doe",
		Organizations: []auth.Organization{
			{
				Name:     "example",
				Provider: "github",
			},
		},
	}

	err := db.Save(&user).Error
	require.NoError(t, err)

	currentMemberships, err := store.GetOrganizationMembershipsOf(context.Background(), user.ID)
	require.NoError(t, err)

	require.Len(t, currentMemberships, 1, "user is expected to be the member of one organization")
	assert.Equal(t, user.Organizations[0].Name, currentMemberships[0].Organization.Name)
	assert.Equal(t, auth.RoleMember, currentMemberships[0].Role)
}

func TestGormOrganizationStore_RemoveUserFromOrganization(t *testing.T) {
	db := setUpDatabase(t)
	store := NewGormOrganizationStore(db)

	user := auth.User{
		Name:  "John Doe",
		Email: "john.doe@example.com",
		Login: "john.doe",
		Organizations: []auth.Organization{
			{
				Name:     "example",
				Provider: "github",
			},
			{
				Name:     "remove-from-this",
				Provider: "github",
			},
		},
	}

	err := db.Save(&user).Error
	require.NoError(t, err)

	err = store.RemoveUserFromOrganization(context.Background(), user.Organizations[1].ID, user.ID)
	require.NoError(t, err)

	var organizations []auth.Organization

	err = db.Model(user).Association("Organizations").Find(&organizations).Error
	require.NoError(t, err)

	require.Len(t, organizations, 1, "user is expected to be the member of one organization")
	assert.Equal(t, user.Organizations[0].Name, organizations[0].Name)
}

func TestGormOrganizationStore_ApplyUserMembership(t *testing.T) {
	// This causes `concurrent map write` issues during tests
	// t.Parallel()

	t.Run("existing", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		user := auth.User{
			Name:  "John Doe",
			Email: "john.doe@example.com",
			Login: "john.doe",
			Organizations: []auth.Organization{
				{
					Name:     "example",
					Provider: "github",
				},
			},
		}

		err := db.Save(&user).Error
		require.NoError(t, err)

		err = store.ApplyUserMembership(context.Background(), user.Organizations[0].ID, user.ID, auth.RoleAdmin)
		require.NoError(t, err)

		var userOrganization auth.UserOrganization

		err = db.
			Where(auth.UserOrganization{UserID: user.ID, OrganizationID: user.Organizations[0].ID}).
			First(&userOrganization).
			Error
		require.NoError(t, err)

		assert.Equal(t, userOrganization.Role, auth.RoleAdmin, "user is expected to be an admin")
	})

	t.Run("existing_no_change", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		user := auth.User{
			Name:  "John Doe",
			Email: "john.doe@example.com",
			Login: "john.doe",
			Organizations: []auth.Organization{
				{
					Name:     "example",
					Provider: "github",
				},
			},
		}

		err := db.Save(&user).Error
		require.NoError(t, err)

		err = store.ApplyUserMembership(context.Background(), user.Organizations[0].ID, user.ID, auth.RoleMember)
		require.NoError(t, err)

		var userOrganization auth.UserOrganization

		err = db.
			Where(auth.UserOrganization{UserID: user.ID, OrganizationID: user.Organizations[0].ID}).
			First(&userOrganization).
			Error
		require.NoError(t, err)

		assert.Equal(t, userOrganization.Role, auth.RoleMember, "user is expected to be a member")
	})

	t.Run("new", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		user := auth.User{
			Name:  "John Doe",
			Email: "john.doe@example.com",
			Login: "john.doe",
		}

		organization := auth.Organization{
			Name:     "example",
			Provider: "github",
		}

		err := db.Save(&user).Error
		require.NoError(t, err)

		err = db.Save(&organization).Error
		require.NoError(t, err)

		err = store.ApplyUserMembership(context.Background(), organization.ID, user.ID, auth.RoleAdmin)
		require.NoError(t, err)

		var userOrganization auth.UserOrganization

		err = db.
			Where(auth.UserOrganization{UserID: user.ID, OrganizationID: organization.ID}).
			First(&userOrganization).
			Error
		require.NoError(t, err)

		assert.Equal(t, userOrganization.Role, auth.RoleAdmin, "user is expected to be an admin")
	})
}

func TestGormOrganizationStore_FindUserRole(t *testing.T) {
	// This causes `concurrent map write` issues during tests
	// t.Parallel()

	t.Run("admin", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		user := auth.User{
			Name:  "John Doe",
			Email: "john.doe@example.com",
			Login: "john.doe",
			Organizations: []auth.Organization{
				{
					Name:     "example",
					Provider: "github",
				},
			},
		}

		err := db.Save(&user).Error
		require.NoError(t, err)

		err = store.ApplyUserMembership(context.Background(), user.Organizations[0].ID, user.ID, auth.RoleAdmin)
		require.NoError(t, err)

		role, member, err := store.FindUserRole(context.Background(), user.Organizations[0].ID, user.ID)
		require.NoError(t, err)

		assert.True(t, member, "user is expected to be a member of the organization")
		assert.Equal(t, role, auth.RoleAdmin, "user is expected to be an admin")
	})

	t.Run("not_a_member", func(t *testing.T) {
		db := setUpDatabase(t)
		store := NewGormOrganizationStore(db)

		user := auth.User{
			Name:  "John Doe",
			Email: "john.doe@example.com",
			Login: "john.doe",
		}

		organization := auth.Organization{
			Name:     "example",
			Provider: "github",
		}

		err := db.Save(&user).Error
		require.NoError(t, err)

		err = db.Save(&organization).Error
		require.NoError(t, err)

		_, member, err := store.FindUserRole(context.Background(), organization.ID, user.ID)
		require.NoError(t, err)

		assert.False(t, member, "user is not expected to be a member of the organization")
	})
}
