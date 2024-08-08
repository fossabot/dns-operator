//go:build unit

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/external-dns/endpoint"
	gatewayapiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	IPAddressOne = "127.0.0.1"
	IPAddressTwo = "127.0.0.2"
	TestHostname = "pat.the.cat"
)

var (
	TestGateway   *gatewayapiv1.Gateway
	TestDnsRecord *DNSRecord
	TestListener  gatewayapiv1.Listener
	TestRouting   *Routing

	domain      = "example.com"
	clusterHash = "2q5hyv"
	gwHash      = "oe3k96"
	defaultGeo  = "IE"
	clusterID   = "fbf71c44-6b37-4962-ace6-801912e769be"
)

var _ = Describe("DnsrecordEndpoints", func() {
	BeforeEach(func() {
		// reset all structs
		TestGateway = &gatewayapiv1.Gateway{}
		TestDnsRecord = &DNSRecord{}
		TestListener = gatewayapiv1.Listener{}
		TestRouting = &Routing{}
	})
	Context("Success scenarios", func() {
		Context("Simple routing Strategy", func() {
			BeforeEach(func() {
				TestGateway = &gatewayapiv1.Gateway{
					Status: gatewayapiv1.GatewayStatus{
						Addresses: []gatewayapiv1.GatewayStatusAddress{
							{
								Type:  ptr.To(gatewayapiv1.IPAddressType),
								Value: IPAddressOne,
							},
							{
								Type:  ptr.To(gatewayapiv1.IPAddressType),
								Value: IPAddressTwo,
							},
						},
					},
				}
				TestDnsRecord = &DNSRecord{
					Spec: DNSRecordSpec{
						Endpoints: []*endpoint.Endpoint{
							{
								DNSName:       HostOne(domain),
								Targets:       []string{IPAddressOne, IPAddressTwo},
								RecordType:    "A",
								SetIdentifier: "",
								RecordTTL:     DefaultTTL,
							},
						},
					},
				}
				TestRouting, _ = NewRoutingBuilder().WithSimpleStrategy().Build()
			})
			It("Should generate endpoint", func() {
				TestListener = gatewayapiv1.Listener{
					Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
				}
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoints).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"DNSName":       Equal(HostOne(domain)),
						"Targets":       ContainElements(IPAddressOne, IPAddressTwo),
						"RecordType":    Equal("A"),
						"SetIdentifier": Equal(""),
						"RecordTTL":     Equal(endpoint.TTL(60)),
					})),
				))
			})
			It("Should generate wildcard endpoint", func() {
				TestListener := gatewayapiv1.Listener{
					Hostname: ptr.To(gatewayapiv1.Hostname(HostWildcard(domain))),
				}
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoints).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"DNSName":       Equal(HostWildcard(domain)),
						"Targets":       ContainElements(IPAddressOne, IPAddressTwo),
						"RecordType":    Equal("A"),
						"SetIdentifier": Equal(""),
						"RecordTTL":     Equal(endpoint.TTL(60)),
					})),
				))
			})
			It("Should generate hostname endpoint", func() {
				TestGateway.Status.Addresses = []gatewayapiv1.GatewayStatusAddress{
					{
						Type:  ptr.To(gatewayapiv1.HostnameAddressType),
						Value: TestHostname,
					},
				}
				TestListener = gatewayapiv1.Listener{
					Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
				}
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoints).To(ConsistOf(
					PointTo(MatchFields(IgnoreExtras, Fields{
						"DNSName":       Equal(HostOne(domain)),
						"Targets":       ContainElement(TestHostname),
						"RecordType":    Equal("CNAME"),
						"SetIdentifier": Equal(""),
						"RecordTTL":     Equal(endpoint.TTL(60)),
					})),
				))

			})
		})
		Context("Load-balanced routing strategy", func() {
			BeforeEach(func() {
				TestGateway = &gatewayapiv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							LabelLBAttributeGeoCode: defaultGeo,
						},
					},
					Status: gatewayapiv1.GatewayStatus{
						Addresses: []gatewayapiv1.GatewayStatusAddress{
							{
								Type:  ptr.To(gatewayapiv1.IPAddressType),
								Value: IPAddressOne,
							},
							{
								Type:  ptr.To(gatewayapiv1.IPAddressType),
								Value: IPAddressTwo,
							},
						},
					},
				}
				TestRouting, _ = NewRoutingBuilder().WithLoadBalancedStrategy(clusterID, defaultGeo, 120).Build()
			})
			Context("With matching geo", func() {
				It("Should generate endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostOne(domain), []string{IPAddressOne, IPAddressTwo})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"Targets":       ConsistOf(IPAddressOne, IPAddressTwo),
							"RecordType":    Equal("A"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(60)),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("ie.klb.test." + domain),
							"Targets":          ConsistOf(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "120"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb.test." + domain),
							"Targets":          ConsistOf("ie.klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(defaultGeo),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: defaultGeo}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb.test." + domain),
							"Targets":          ConsistOf("ie.klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("default"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: "*"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostOne(domain)),
							"Targets":       ConsistOf("klb.test." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))

				})
				It("Should generate wildcard endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostWildcard(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostWildcard(domain), []string{IPAddressOne, IPAddressTwo})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"Targets":       ConsistOf(IPAddressOne, IPAddressTwo),
							"RecordType":    Equal("A"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(60)),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("ie.klb." + domain),
							"Targets":          ConsistOf(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "120"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb." + domain),
							"Targets":          ConsistOf("ie.klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("default"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: "*"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb." + domain),
							"Targets":          ConsistOf("ie.klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(defaultGeo),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: defaultGeo}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostWildcard(domain)),
							"Targets":       ConsistOf("klb." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))
				})
			})

			Context("Load-balanced routing strategy with non-matching geo", func() {
				BeforeEach(func() {
					TestGateway.Labels[LabelLBAttributeGeoCode] = "ES"
				})
				It("Should generate endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostOne(domain), []string{IPAddressOne, IPAddressTwo})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"Targets":       ConsistOf(IPAddressOne, IPAddressTwo),
							"RecordType":    Equal("A"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(60)),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("es.klb.test." + domain),
							"Targets":          ConsistOf(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "120"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb.test." + domain),
							"Targets":          ConsistOf("es.klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("ES"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: "ES"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostOne(domain)),
							"Targets":       ConsistOf("klb.test." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))

				})
				It("Should generate wildcard endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostWildcard(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostWildcard(domain), []string{IPAddressOne, IPAddressTwo})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"Targets":       ConsistOf(IPAddressOne, IPAddressTwo),
							"RecordType":    Equal("A"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(60)),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("es.klb." + domain),
							"Targets":          ConsistOf(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "120"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb." + domain),
							"Targets":          ConsistOf("es.klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("ES"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: "ES"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostWildcard(domain)),
							"Targets":       ConsistOf("klb." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))
				})
			})

			Context("Load-balanced routing strategy with custom weights", func() {
				BeforeEach(func() {
					TestRouting, _ = NewRoutingBuilder().WithLoadBalancedStrategy(clusterID, defaultGeo, 120).
						WithCustomWeights([]CustomWeight{
							{
								Selector: metav1.LabelSelector{
									MatchLabels: map[string]string{
										"kuadrant.io/my-custom-weight-attr": "FOO",
									},
								},
								Weight: 100,
							},
						}).Build()
					TestGateway.Labels["kuadrant.io/my-custom-weight-attr"] = "FOO"

				})
				It("Should generate endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostOne(domain), []string{IPAddressOne, IPAddressTwo})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"Targets":       ConsistOf(IPAddressOne, IPAddressTwo),
							"RecordType":    Equal("A"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(60)),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("ie.klb.test." + domain),
							"Targets":          ConsistOf(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(clusterHash + "-" + gwHash + "." + "klb.test." + domain),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "100"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb.test." + domain),
							"Targets":          ConsistOf("ie.klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(defaultGeo),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: defaultGeo}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb.test." + domain),
							"Targets":          ConsistOf("ie.klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("default"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: "*"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostOne(domain)),
							"Targets":       ConsistOf("klb.test." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))

				})
				It("Should generate wildcard endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostWildcard(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostWildcard(domain), []string{IPAddressOne, IPAddressTwo})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"Targets":       ConsistOf(IPAddressOne, IPAddressTwo),
							"RecordType":    Equal("A"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(60)),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("ie.klb." + domain),
							"Targets":          ConsistOf(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(clusterHash + "-" + gwHash + "." + "klb." + domain),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "100"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb." + domain),
							"Targets":          ConsistOf("ie.klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("default"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: "*"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb." + domain),
							"Targets":          ConsistOf("ie.klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(defaultGeo),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "geo-code", Value: defaultGeo}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostWildcard(domain)),
							"Targets":       ConsistOf("klb." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))
				})

			})

			Context("With missing geo label on Gateway and hostname address", func() {
				BeforeEach(func() {
					TestGateway.Labels = map[string]string{}
					TestGateway.Status.Addresses = []gatewayapiv1.GatewayStatusAddress{
						{
							Type:  ptr.To(gatewayapiv1.HostnameAddressType),
							Value: TestHostname,
						},
					}
				})

				It("Should generate endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostOne(domain), []string{TestHostname})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("default.klb.test." + domain),
							"Targets":          ConsistOf(TestHostname),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(TestHostname),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "120"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb.test." + domain),
							"Targets":          ConsistOf("default.klb.test." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("default"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": BeNil(),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostOne(domain)),
							"Targets":       ConsistOf("klb.test." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))

				})
				It("Should generate wildcard endpoints", func() {
					TestListener = gatewayapiv1.Listener{
						Hostname: ptr.To(gatewayapiv1.Hostname(HostWildcard(domain))),
					}
					endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
					Expect(err).NotTo(HaveOccurred())
					Expect(EndpointsTraversable(endpoints, HostWildcard(domain), []string{TestHostname})).To(BeTrue())
					Expect(endpoints).To(ConsistOf(
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("default.klb." + domain),
							"Targets":          ConsistOf(TestHostname),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal(TestHostname),
							"RecordTTL":        Equal(endpoint.TTL(60)),
							"ProviderSpecific": Equal(endpoint.ProviderSpecific{{Name: "weight", Value: "120"}}),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":          Equal("klb." + domain),
							"Targets":          ConsistOf("default.klb." + domain),
							"RecordType":       Equal("CNAME"),
							"SetIdentifier":    Equal("default"),
							"RecordTTL":        Equal(endpoint.TTL(300)),
							"ProviderSpecific": BeNil(),
						})),
						PointTo(MatchFields(IgnoreExtras, Fields{
							"DNSName":       Equal(HostWildcard(domain)),
							"Targets":       ConsistOf("klb." + domain),
							"RecordType":    Equal("CNAME"),
							"SetIdentifier": Equal(""),
							"RecordTTL":     Equal(endpoint.TTL(300)),
						})),
					))
				})
			})
		})
	})

	Context("Failure scenarios", func() {
		BeforeEach(func() {
			// create valid set of inputs for lb strategy with custom weights.
			TestGateway = &gatewayapiv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelLBAttributeGeoCode:             defaultGeo,
						"kuadrant.io/my-custom-weight-attr": "FOO",
					},
				},
				Status: gatewayapiv1.GatewayStatus{
					Addresses: []gatewayapiv1.GatewayStatusAddress{
						{
							Type:  ptr.To(gatewayapiv1.IPAddressType),
							Value: IPAddressOne,
						},
						{
							Type:  ptr.To(gatewayapiv1.IPAddressType),
							Value: IPAddressTwo,
						},
					},
				},
			}
			TestDnsRecord = &DNSRecord{}
			TestListener = gatewayapiv1.Listener{
				Hostname: ptr.To(gatewayapiv1.Hostname(HostOne(domain))),
			}
			TestRouting, _ = NewRoutingBuilder().WithLoadBalancedStrategy(clusterID, defaultGeo, 120).
				WithCustomWeights([]CustomWeight{
					{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kuadrant.io/my-custom-weight-attr": "FOO",
							},
						},
						Weight: 100,
					},
				}).Build()
		})
		It("Should not accept unknown strategy", func() {
			TestRouting.Strategy = "cat"
			endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
			Expect(endpoints).To(BeNil())
			Expect(err.Error()).To(ContainSubstring(ErrUnknownRoutingStrategy.Error()))
		})
		It("Should not allow for nil listener", func() {
			TestListener.Hostname = nil
			endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
			Expect(endpoints).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("listener hostname is empty"))
		})
		It("Should not allow for nil current dns record", func() {
			endpoints, err := GenerateEndpoints(TestGateway, nil, TestListener, TestRouting)
			Expect(endpoints).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("require current endpoints"))
		})
		Context("Should not allow for invalid routing", func() {
			It("with missing cluster id", func() {
				TestRouting.ClusterID = ""
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(endpoints).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("cluster ID is required"))
			})
			It("with missing default weight", func() {
				TestRouting.DefaultWeight = 0
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(endpoints).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("default weight is required"))
			})
			It("with missing default geo", func() {
				TestRouting.DefaultGeoCode = ""
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(endpoints).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("default geocode is required"))
			})
			It("with zero custom weight", func() {
				TestRouting.CustomWeights[0].Weight = 0
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(endpoints).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("custom weight cannot be zero"))
			})
			It("with missing selector on custom weight", func() {
				TestRouting.CustomWeights[0].Selector = metav1.LabelSelector{}
				endpoints, err := GenerateEndpoints(TestGateway, TestDnsRecord, TestListener, TestRouting)
				Expect(endpoints).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("custom weight must define non-empty selector"))
			})
		})

	})
})
