package radom

import (
	"context"
	"testing"

	uuid "github.com/satori/go.uuid"
	should "github.com/stretchr/testify/assert"
	must "github.com/stretchr/testify/require"
)

func TestParseEvent(t *testing.T) {
	type tcGiven struct {
		rawEvent string
	}

	type tcExpected struct {
		event   *Event
		mustErr must.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "new_subscription",
			given: tcGiven{
				rawEvent: `{
				  "eventType": "newSubscription",
				  "eventData": {
					"newSubscription": {
					  "subscriptionId": "54453f86-8cfa-4eee-8818-050fc61f560b"
					}
				  },
				  "radomData": {
					"checkoutSession": {
					  "checkoutSessionId": "71da4f76-0ac7-47ee-bb51-9d9577232245",
					  "metadata": [
						{
						  "key": "brave_order_id",
						  "value": "b5269191-1d8d-4934-b105-3221da010222"
						}
					  ]
					}
				  }
				}`,
			},
			exp: tcExpected{
				event: &Event{
					EventType: "newSubscription",
					EventData: &EventData{
						New: &NewSubscription{
							SubscriptionID: uuid.FromStringOrNil("54453f86-8cfa-4eee-8818-050fc61f560b"),
						},
					},
					RadomData: &Data{
						CheckoutSession: &CheckoutSession{
							CheckoutSessionID: "71da4f76-0ac7-47ee-bb51-9d9577232245",
							Metadata: []Metadata{
								{
									Key:   "brave_order_id",
									Value: "b5269191-1d8d-4934-b105-3221da010222",
								},
							},
						},
					},
				},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_payment",
			given: tcGiven{
				rawEvent: `{
				  "eventType": "subscriptionPayment",
				  "eventData": {
					"subscriptionPayment": {
					  "radomData": {
						"subscription": {
						  "subscriptionId": "2eb2fcf0-8c73-4e2b-9e94-50f76f8caabe"
						}
					  }
					}
				  },
				  "radomData": {
					"subscription": {
					  "subscriptionId": "2eb2fcf0-8c73-4e2b-9e94-50f76f8caabe"
					}
				  }
				}`,
			},
			exp: tcExpected{
				event: &Event{
					EventType: "subscriptionPayment",
					EventData: &EventData{
						Payment: &SubscriptionPayment{
							RadomData: &Data{
								Subscription: &Subscription{
									SubscriptionID: uuid.FromStringOrNil("2eb2fcf0-8c73-4e2b-9e94-50f76f8caabe"),
								},
							},
						},
					},
					RadomData: &Data{
						Subscription: &Subscription{
							SubscriptionID: uuid.FromStringOrNil("2eb2fcf0-8c73-4e2b-9e94-50f76f8caabe"),
						},
					},
				},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_cancelled",
			given: tcGiven{
				rawEvent: `{
				  "eventType": "subscriptionCancelled",
				  "eventData": {
					"subscriptionCancelled": {
					  "subscriptionId": "56786d4e-a994-4392-952a-a648a0d2870a"
					}
				  }
				}`,
			},
			exp: tcExpected{
				event: &Event{
					EventType: "subscriptionCancelled",
					EventData: &EventData{
						Cancelled: &SubscriptionCancelled{
							SubscriptionID: uuid.FromStringOrNil("56786d4e-a994-4392-952a-a648a0d2870a"),
						},
					},
				},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_expired",
			given: tcGiven{
				rawEvent: `{
				  "eventType": "subscriptionExpired",
				  "eventData": {
					"subscriptionExpired": {
					  "subscriptionId": "56786d4e-a994-4392-952a-a648a0d2870a"
					}
				  }
				}`,
			},
			exp: tcExpected{
				event: &Event{
					EventType: "subscriptionExpired",
					EventData: &EventData{
						Expired: &SubscriptionExpired{
							SubscriptionID: uuid.FromStringOrNil("56786d4e-a994-4392-952a-a648a0d2870a"),
						},
					},
				},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "unknown_event",
			given: tcGiven{
				rawEvent: `{
				  "eventType": "unknownEvent",
				  "eventData": {
					"unknownEvent": {
					  "subscriptionId": "56786d4e-a994-4392-952a-a648a0d2870a"
					}
				  }
				}`,
			},
			exp: tcExpected{
				event: &Event{
					EventType: "unknownEvent",
					EventData: &EventData{},
				},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "unknown_event_data",
			given: tcGiven{
				rawEvent: `{
				  "eventType": "unknownEvent",
				  "unknownEventData": {
					"unknownEvent": {
					  "subscriptionId": "56786d4e-a994-4392-952a-a648a0d2870a"
					}
				  }
				}`,
			},
			exp: tcExpected{
				event: &Event{
					EventType: "unknownEvent",
				},
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual, err := ParseEvent([]byte(tc.given.rawEvent))
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.event, actual)
		})
	}
}

func TestEvent_OrderID(t *testing.T) {
	type tcGiven struct {
		event Event
	}

	type tcExpected struct {
		oid     uuid.UUID
		mustErr must.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "no_event_data",
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrUnsupportedEvent)
				},
			},
		},

		{
			name: "unsupported_event",
			given: tcGiven{
				event: Event{
					EventData: &EventData{},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrUnsupportedEvent)
				},
			},
		},

		{
			name: "no_radom_data",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrNoCheckoutSessionData)
				},
			},
		},

		{
			name: "no_checkout_data",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
					RadomData: &Data{},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrNoCheckoutSessionData)
				},
			},
		},

		{
			name: "invalid_id",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
					RadomData: &Data{
						CheckoutSession: &CheckoutSession{
							Metadata: []Metadata{
								{
									Key:   "brave_order_id",
									Value: "invalid_uuid",
								},
							},
						},
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorContains(t, err, "incorrect UUID")
				},
			},
		},

		{
			name: "order_id_found",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
					RadomData: &Data{
						CheckoutSession: &CheckoutSession{
							Metadata: []Metadata{
								{
									Key:   "brave_order_id",
									Value: "053e0244-4e37-48c3-8539-49952ec73f37",
								},
							},
						},
					},
				},
			},
			exp: tcExpected{
				oid: uuid.FromStringOrNil("053e0244-4e37-48c3-8539-49952ec73f37"),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "order_id_not_found",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
					RadomData: &Data{
						CheckoutSession: &CheckoutSession{
							Metadata: []Metadata{
								{
									Key:   "some_key",
									Value: "053e0244-4e37-48c3-8539-49952ec73f37",
								},
							},
						},
					},
				},
			},
			exp: tcExpected{
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrBraveOrderIDNotFound)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {

			actual, err := tc.given.event.OrderID()
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.oid, actual)
		})
	}
}

func TestEvent_SubID(t *testing.T) {
	type tcGiven struct {
		event Event
	}

	type tcExpected struct {
		sid     uuid.UUID
		mustErr must.ErrorAssertionFunc
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "new_subscription",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{
							SubscriptionID: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
						},
					},
				},
			},
			exp: tcExpected{
				sid: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_payment_no_radom_data",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Payment: &SubscriptionPayment{},
					},
				},
			},
			exp: tcExpected{
				sid: uuid.Nil,
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrSubscriptionIDNotFound)
				},
			},
		},

		{
			name: "subscription_payment_no_subscription",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Payment: &SubscriptionPayment{
							RadomData: &Data{},
						},
					},
				},
			},
			exp: tcExpected{
				sid: uuid.Nil,
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrSubscriptionIDNotFound)
				},
			},
		},

		{
			name: "subscription_payment",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Payment: &SubscriptionPayment{
							RadomData: &Data{
								Subscription: &Subscription{
									SubscriptionID: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
								},
							},
						},
					},
				},
			},
			exp: tcExpected{
				sid: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_cancelled",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Cancelled: &SubscriptionCancelled{
							SubscriptionID: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
						},
					},
				},
			},
			exp: tcExpected{
				sid: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name: "subscription_expired",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Expired: &SubscriptionExpired{
							SubscriptionID: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
						},
					},
				},
			},
			exp: tcExpected{
				sid: uuid.FromStringOrNil("d14c5b2e-b719-4504-b034-86e74a932295"),
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.NoError(t, err)
				},
			},
		},

		{
			name:  "no_event_data",
			given: tcGiven{event: Event{}},
			exp: tcExpected{
				sid: uuid.Nil,
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrUnsupportedEvent)
				},
			},
		},

		{
			name: "unknown_event",
			given: tcGiven{
				event: Event{
					EventData: &EventData{},
				},
			},
			exp: tcExpected{
				sid: uuid.Nil,
				mustErr: func(t must.TestingT, err error, i ...interface{}) {
					must.ErrorIs(t, err, ErrUnsupportedEvent)
				},
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {

			actual, err := tc.given.event.SubID()
			tc.exp.mustErr(t, err)

			should.Equal(t, tc.exp.sid, actual)
		})
	}
}

func TestEvent_IsNewSub(t *testing.T) {
	type tcGiven struct {
		event Event
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "new_subscription",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
				},
			},
			exp: true,
		},

		{
			name: "not_new_subscription",
			given: tcGiven{
				event: Event{
					EventData: &EventData{},
				},
			},
			exp: false,
		},

		{
			name: "not_new_subscription_event_data",
			exp:  false,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.event.IsNewSub()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestEvent_ShouldRenew(t *testing.T) {
	type tcGiven struct {
		event Event
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "subscription_payment",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Payment: &SubscriptionPayment{},
					},
				},
			},
			exp: true,
		},

		{
			name: "not_subscription_payment",
			given: tcGiven{
				event: Event{
					EventData: &EventData{},
				},
			},
			exp: false,
		},

		{
			name: "not_subscription_payment_event_data",
			exp:  false,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.event.ShouldRenew()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestEvent_ShouldCancel(t *testing.T) {
	type tcGiven struct {
		event *Event
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "event_nil",
			exp:  false,
		},

		{
			name: "event_data_nil",
			given: tcGiven{
				event: &Event{},
			},
			exp: false,
		},

		{
			name: "subscription_cancelled",
			given: tcGiven{
				event: &Event{
					EventData: &EventData{
						Cancelled: &SubscriptionCancelled{},
					},
				},
			},
			exp: true,
		},

		{
			name: "subscription_expired",
			given: tcGiven{
				event: &Event{
					EventData: &EventData{
						Expired: &SubscriptionExpired{},
					},
				},
			},
			exp: true,
		},

		{
			name: "unknown_action",
			given: tcGiven{
				event: &Event{
					EventData: &EventData{},
				},
			},
			exp: false,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.event.ShouldCancel()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestEvent_ShouldProcess(t *testing.T) {
	type tcGiven struct {
		event Event
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   bool
	}

	tests := []testCase{
		{
			name: "new_subscription",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
				},
			},
			exp: true,
		},

		{
			name: "subscription_payment",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Payment: &SubscriptionPayment{},
					},
				},
			},
			exp: true,
		},

		{
			name: "subscription_cancelled",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Cancelled: &SubscriptionCancelled{},
					},
				},
			},
			exp: true,
		},

		{
			name: "subscription_expired",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Expired: &SubscriptionExpired{},
					},
				},
			},
			exp: true,
		},

		{
			name: "not_should_process",
			exp:  false,
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.event.ShouldProcess()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestEvent_Effect(t *testing.T) {
	type tcGiven struct {
		event Event
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   string
	}

	tests := []testCase{
		{
			name: "new",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						New: &NewSubscription{},
					},
				},
			},
			exp: "new",
		},

		{
			name: "renew",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Payment: &SubscriptionPayment{},
					},
				},
			},
			exp: "renew",
		},

		{
			name: "cancel",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Cancelled: &SubscriptionCancelled{},
					},
				},
			},
			exp: "cancel",
		},

		{
			name: "expired",
			given: tcGiven{
				event: Event{
					EventData: &EventData{
						Expired: &SubscriptionExpired{},
					},
				},
			},
			exp: "cancel",
		},

		{
			name: "skip",
			exp:  "skip",
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			actual := tc.given.event.Effect()
			should.Equal(t, tc.exp, actual)
		})
	}
}

func TestMessageAuthenticator_Authenticate(t *testing.T) {
	type tcGiven struct {
		mAuth MessageAuthenticator
		token string
	}

	type tcExpected struct {
		err error
	}

	type testCase struct {
		name  string
		given tcGiven
		exp   tcExpected
	}

	tests := []testCase{
		{
			name: "disabled",
			given: tcGiven{
				mAuth: MessageAuthenticator{},
			},
			exp: tcExpected{
				err: ErrDisabled,
			},
		},

		{
			name: "verification_key_empty",
			given: tcGiven{
				mAuth: MessageAuthenticator{
					cfg: MessageAuthConfig{
						Enabled: true,
						Token:   []byte("token"),
					},
				},
			},
			exp: tcExpected{
				err: ErrVerificationKeyEmpty,
			},
		},

		{
			name: "verification_key_invalid",
			given: tcGiven{
				mAuth: MessageAuthenticator{
					cfg: MessageAuthConfig{
						Enabled: true,
						Token:   []byte("token_1"),
					},
				},
				token: "token_2",
			},
			exp: tcExpected{
				err: ErrVerificationKeyInvalid,
			},
		},

		{
			name: "success",
			given: tcGiven{
				mAuth: MessageAuthenticator{
					cfg: MessageAuthConfig{
						Enabled: true,
						Token:   []byte("token_1"),
					},
				},
				token: "token_1",
			},
		},
	}

	for i := range tests {
		tc := tests[i]

		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			actual := tc.given.mAuth.Authenticate(ctx, tc.given.token)
			should.ErrorIs(t, tc.exp.err, actual)
		})
	}
}
