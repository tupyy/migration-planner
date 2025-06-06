package store_test

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/config"
	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

const (
	insertSourceStm           = "INSERT INTO sources (id, name, username, org_id) VALUES ('%s', '%s', '%s', '%s');"
	insertSourceOnPremisesStm = "INSERT INTO sources (id, name, username, org_id, on_premises) VALUES ('%s','%s', '%s', '%s', TRUE);"
	insertLabelStm            = "INSERT INTO labels (key, value, source_id) VALUES ('%s', '%s', '%s');"
)

var _ = Describe("source store", Ordered, func() {
	var (
		s      store.Store
		gormdb *gorm.DB
	)

	BeforeAll(func() {
		cfg, err := config.New()
		Expect(err).To(BeNil())
		db, err := store.InitDB(cfg)
		Expect(err).To(BeNil())

		s = store.NewStore(db)
		gormdb = db
	})

	AfterAll(func() {
		s.Close()
	})

	Context("list", func() {
		It("successfully list all the sources", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source1", "user2", "org_id_2"))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(2))
		})

		It("successfully list all the sources -- with one agent", func() {
			sourceID := uuid.NewString()
			agentID := uuid.NewString()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))
			Expect(sources[0].Agents).To(HaveLen(1))
			Expect(sources[0].Agents[0].ID.String()).To(Equal(agentID))
		})

		It("successfully list all the sources -- with multiple agents", func() {
			sourceID := uuid.NewString()
			agentID := uuid.NewString()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, uuid.NewString(), "not-connected", "status-info-1", "cred_url-1", sourceID))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))
			Expect(sources[0].Agents).To(HaveLen(2))
		})

		It("list all sources -- no sources", func() {
			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter())
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(0))
		})

		It("successfully list the user's sources", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source2", "user", "user"))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter().ByUsername("admin"))
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))
			Expect(sources[0].Username).To(Equal("admin"))
		})

		It("successfully list the source on prem", func() {
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source1", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source2", "admin", "admin"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceOnPremisesStm, uuid.NewString(), "source3", "admin", "admin"))
			Expect(tx.Error).To(BeNil())

			sources, err := s.Source().List(context.TODO(), store.NewSourceQueryFilter().ByUsername("admin").ByOnPremises(false))
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(2))

			sources, err = s.Source().List(context.TODO(), store.NewSourceQueryFilter().ByUsername("admin").ByOnPremises(true))
			Expect(err).To(BeNil())
			Expect(sources).To(HaveLen(1))
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from agents;")
			gormdb.Exec("DELETE from sources;")
		})
	})

	Context("get", func() {
		It("successfully get a source", func() {
			id := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, id, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			agentID := uuid.New()
			tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", id.String()))
			Expect(tx.Error).To(BeNil())

			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), id)
			Expect(err).To(BeNil())
			Expect(source).ToNot(BeNil())
			Expect(source.Agents).To(HaveLen(1))
			Expect(source.Agents[0].ID.String()).To(Equal(agentID.String()))
		})

		It("failed get a source -- source does not exists", func() {
			id := uuid.New()
			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), id)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("record not found"))
			Expect(source).To(BeNil())
		})

		AfterEach(func() {
			gormdb.Exec("DELETE from agents;")
			gormdb.Exec("DELETE from sources;")
		})

		Context("create", func() {
			It("successfully creates one source", func() {
				sourceID := uuid.New()
				m := model.Source{
					ID:       sourceID,
					Username: "admin",
					OrgID:    "org",
				}
				source, err := s.Source().Create(context.TODO(), m)
				Expect(err).To(BeNil())
				Expect(source).NotTo(BeNil())

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("failed to create source with the same name in org namespace", func() {
				sourceID := uuid.New()
				m := model.Source{
					ID:       sourceID,
					Username: "admin",
					OrgID:    "org",
				}
				source, err := s.Source().Create(context.TODO(), m)
				Expect(err).To(BeNil())
				Expect(source).NotTo(BeNil())

				var count int
				tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))

				m = model.Source{
					ID:       uuid.New(),
					Username: "admin",
					OrgID:    "org",
				}
				_, err = s.Source().Create(context.TODO(), m)
				Expect(err).ToNot(BeNil())
			})

			AfterEach(func() {
				gormdb.Exec("DELETE from agents;")
				gormdb.Exec("DELETE from sources;")
			})
		})

		Context("delete", func() {
			It("successfully delete a source", func() {
				id := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, id, "source1", "user1", "org_id_1"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source2", "user1", "org_id_1"))
				Expect(tx.Error).To(BeNil())

				err := s.Source().Delete(context.TODO(), id)
				Expect(err).To(BeNil())

				count := 2
				tx = gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(1))
			})

			It("successfully delete all sources", func() {
				id := uuid.New()
				agentID := uuid.New()
				tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, id, "source1", "user1", "org_id_1"))
				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertAgentStm, agentID, "not-connected", "status-info-1", "cred_url-1", id))

				Expect(tx.Error).To(BeNil())
				tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, uuid.NewString(), "source2", "user1", "org_id_1"))
				Expect(tx.Error).To(BeNil())

				err := s.Source().DeleteAll(context.TODO())
				Expect(err).To(BeNil())

				count := 2
				tx = gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))

				count = 1
				tx = gormdb.Raw("SELECT COUNT(*) FROM agents;").Scan(&count)
				Expect(tx.Error).To(BeNil())
				Expect(count).To(Equal(0))
			})

			AfterEach(func() {
				gormdb.Exec("DELETE from agents;")
				gormdb.Exec("DELETE from sources;")
			})
		})
	})

	Context("labels", func() {
		It("get successfully a source with labels", func() {
			sourceID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			// insert two labels
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key1", "value1", sourceID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key2", "value2", sourceID))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(source.Labels).To(HaveLen(2))
			Expect([]string{"key1", "key2"}).To(ContainElement(source.Labels[0].Key))
			Expect([]string{"key1", "key2"}).To(ContainElement(source.Labels[1].Key))
			Expect([]string{"value1", "value2"}).To(ContainElement(source.Labels[0].Value))
			Expect([]string{"value1", "value2"}).To(ContainElement(source.Labels[1].Value))
		})

		It("get successfully a source with labels -- two sources", func() {
			sourceID := uuid.New()
			sourceID2 := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID2, "source2", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			// insert labels for both sources
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key1", "value1", sourceID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key2", "value2", sourceID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key2", "value2", sourceID2))
			Expect(tx.Error).To(BeNil())

			source, err := s.Source().Get(context.TODO(), sourceID)
			Expect(err).To(BeNil())
			Expect(source.Labels).To(HaveLen(2))
			Expect([]string{"key1", "key2"}).To(ContainElement(source.Labels[0].Key))
			Expect([]string{"key1", "key2"}).To(ContainElement(source.Labels[1].Key))
			Expect([]string{"value1", "value2"}).To(ContainElement(source.Labels[0].Value))
			Expect([]string{"value1", "value2"}).To(ContainElement(source.Labels[1].Value))
		})

		It("delete labels when source is deleted", func() {
			sourceID := uuid.New()

			tx := gormdb.Exec(fmt.Sprintf(insertSourceStm, sourceID, "source1", "user1", "org_id_1"))
			Expect(tx.Error).To(BeNil())

			// insert two labels
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key1", "value1", sourceID))
			Expect(tx.Error).To(BeNil())
			tx = gormdb.Exec(fmt.Sprintf(insertLabelStm, "key2", "value2", sourceID))
			Expect(tx.Error).To(BeNil())

			err := s.Source().Delete(context.TODO(), sourceID)
			Expect(err).To(BeNil())

			count := 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))
		})

		It("creates successfully a source with labels", func() {
			sourceID := uuid.New()
			m := model.Source{
				ID:       sourceID,
				Username: "admin",
				OrgID:    "org",
				Labels: []model.Label{
					{
						Key:   "key1",
						Value: "value1",
					},
					{
						Key:   "key2",
						Value: "value1",
					},
				},
			}
			source, err := s.Source().Create(context.TODO(), m)
			Expect(err).To(BeNil())
			Expect(source).NotTo(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			count = 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))
		})

		It("updates successfully a source with labels", func() {
			sourceID := uuid.New()
			m := model.Source{
				ID:       sourceID,
				Username: "admin",
				OrgID:    "org",
			}
			source, err := s.Source().Create(context.TODO(), m)
			Expect(err).To(BeNil())
			Expect(source).NotTo(BeNil())

			var count int
			tx := gormdb.Raw("SELECT COUNT(*) FROM sources;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(1))

			count = 1
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(0))

			m = model.Source{
				ID:       sourceID,
				Username: "admin",
				OrgID:    "org",
				Labels: []model.Label{
					{
						Key:   "key1",
						Value: "value1",
					},
					{
						Key:   "key2",
						Value: "value1",
					},
				},
			}
			source, err = s.Source().Update(context.TODO(), m)
			Expect(err).To(BeNil())
			Expect(source).NotTo(BeNil())

			count = 0
			tx = gormdb.Raw("SELECT COUNT(*) FROM labels;").Scan(&count)
			Expect(tx.Error).To(BeNil())
			Expect(count).To(Equal(2))

		})

		AfterEach(func() {
			gormdb.Exec("DELETE from labels;")
			gormdb.Exec("DELETE from sources;")
		})
	})
})
