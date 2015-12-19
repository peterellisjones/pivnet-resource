package pivnet_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"github.com/pivotal-cf-experimental/pivnet-resource/pivnet"
)

const (
	apiPrefix = "/api/v2"
)

var _ = Describe("PivnetClient", func() {
	var (
		server     *ghttp.Server
		client     pivnet.Client
		token      string
		apiAddress string
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		apiAddress = server.URL() + apiPrefix
		token = "my-auth-token"
		client = pivnet.NewClient(apiAddress, token)
	})

	AfterEach(func() {
		server.Close()
	})

	It("has authenticated headers for each request", func() {
		response := fmt.Sprintf(`{"releases": [{"version": "1234"}]}`)

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", apiPrefix+"/products/my-product-id/releases"),
				ghttp.VerifyHeaderKV("Authorization", fmt.Sprintf("Token %s", token)),
				ghttp.RespondWith(http.StatusOK, response),
			),
		)

		_, err := client.ProductVersions("my-product-id")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Get Release", func() {
		It("returns the release based on the name and version", func() {
			response := `{"releases": [{"id": 3, "version": "3.2.1", "_links": {"product_files": {"href":"https://banana.org/cookies/download"}}}]}`

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", apiPrefix+"/products/banana/releases"),
					ghttp.RespondWith(http.StatusOK, response),
				),
			)

			release, err := client.GetRelease("banana", "3.2.1")
			Expect(err).NotTo(HaveOccurred())
			Expect(release.Links.ProductFiles["href"]).To(Equal("https://banana.org/cookies/download"))
		})

		Context("when the requested version is not available but the request is successful", func() {
			It("returns an error", func() {
				response := `{"releases": [{"id": 3, "version": "3.2.1"}]}`

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", apiPrefix+"/products/banana/releases"),
						ghttp.RespondWith(http.StatusOK, response),
					),
				)

				_, err := client.GetRelease("banana", "1.0.0")
				Expect(err).To(MatchError(errors.New("The requested version: 1.0.0 - could not be found")))
			})
		})

		Context("when the server responds with a non-2XX status code", func() {
			It("returns an error", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", apiPrefix+"/products/banana/releases"),
						ghttp.RespondWith(http.StatusTeapot, nil),
					),
				)

				_, err := client.GetRelease("banana", "1.0.0")
				Expect(err).To(MatchError(errors.New("Pivnet returned status code: 418 for the request")))
			})
		})
	})

	Describe("Get Product Files", func() {
		It("returns the product files for the given release", func() {
			response, err := json.Marshal(pivnet.ProductFiles{[]pivnet.ProductFile{
				{ID: 3, AWSObjectKey: "anything", Links: pivnet.Links{Download: map[string]string{"href": "/products/banana/releases/666/product_files/6/download"}}},
				{ID: 4, AWSObjectKey: "something", Links: pivnet.Links{Download: map[string]string{"href": "/products/banana/releases/666/product_files/8/download"}}},
			},
			})
			Expect(err).NotTo(HaveOccurred())

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", apiPrefix+"/products/banana/releases/666/product_files"),
					ghttp.RespondWith(http.StatusOK, response),
				),
			)

			release := pivnet.Release{
				Links: pivnet.Links{
					ProductFiles: map[string]string{"href": apiAddress + "/products/banana/releases/666/product_files"},
				},
			}

			product, err := client.GetProductFiles(release)
			Expect(err).NotTo(HaveOccurred())
			Expect(product.ProductFiles).To(HaveLen(2))

			Expect(product.ProductFiles[0].AWSObjectKey).To(Equal("anything"))
			Expect(product.ProductFiles[1].AWSObjectKey).To(Equal("something"))

			Expect(product.ProductFiles[0].Links.Download["href"]).To(Equal("/products/banana/releases/666/product_files/6/download"))
			Expect(product.ProductFiles[1].Links.Download["href"]).To(Equal("/products/banana/releases/666/product_files/8/download"))
		})

		Context("when the server responds with a non-2XX status code", func() {
			It("returns an error", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", apiPrefix+"/products/banana/releases/666/product_files"),
						ghttp.RespondWith(http.StatusTeapot, nil),
					),
				)
				release := pivnet.Release{
					Links: pivnet.Links{
						ProductFiles: map[string]string{"href": apiAddress + "/products/banana/releases/666/product_files"},
					},
				}

				_, err := client.GetProductFiles(release)
				Expect(err).To(MatchError(errors.New("Pivnet returned status code: 418 for the request")))
			})
		})
	})

	Describe("Product Versions", func() {
		Context("when parsing the url fails with error", func() {
			It("forwards the error", func() {
				badURL := "%%%"
				client = pivnet.NewClient(badURL, token)

				_, err := client.ProductVersions("some product")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("%%%"))
			})
		})

		Context("when making the request fails with error", func() {
			It("forwards the error", func() {
				badURL := "https://not-a-real-url.com"
				client = pivnet.NewClient(badURL, token)

				_, err := client.ProductVersions("some product")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no such host"))
			})
		})

		Context("when a non-200 comes back from Pivnet", func() {
			It("returns an error", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", apiPrefix+"/products/my-product-id/releases"),
						ghttp.RespondWith(http.StatusNotFound, nil),
					),
				)

				_, err := client.ProductVersions("my-product-id")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("Pivnet returned status code: 404 for the request"))
			})
		})

		Context("when the json unmarshalling fails with error", func() {
			It("forwards the error", func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", apiPrefix+"/products/my-product-id/releases"),
						ghttp.RespondWith(http.StatusOK, "%%%"),
					),
				)

				_, err := client.ProductVersions("my-product-id")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid character"))
			})
		})

		It("gets versions", func() {
			productVersion := "v" + strconv.Itoa(rand.Int())
			response := fmt.Sprintf(
				`{"releases": [{"version": %q}, {"version": %q}]}`,
				productVersion, productVersion,
			)

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", apiPrefix+"/products/my-product-id/releases"),
					ghttp.RespondWith(http.StatusOK, response),
				),
			)

			versions, err := client.ProductVersions("my-product-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(server.ReceivedRequests()).Should(HaveLen(1))
			Expect(versions).To(HaveLen(2))
			Expect(versions[0]).Should(Equal(productVersion))
			Expect(versions[1]).Should(Equal(productVersion))
		})
	})
})
