package payment

//func TestPrepare(t *testing.T) {
//	expected := Transaction{
//		IdempotencyKey: uuid.NewV4(),
//		Custodian:      testutils.RandomString(),
//		To:             uuid.NewV4(),
//		Amount:         decimal.New(1, 0),
//	}
//
//	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		require.Equal(t, http.MethodPost, r.Method)
//		require.Equal(t, "/v1/payments/prepare", r.URL.Path)
//
//		// assert we received the expected transaction
//		var transaction Transaction
//		err := json.NewDecoder(r.Body).Decode(&transaction)
//
//		require.NoError(t, err)
//		assert.Equal(t, expected, transaction)
//
//		// return the received transaction
//		w.WriteHeader(http.StatusCreated)
//
//		payload, err := json.Marshal(transaction)
//		require.NoError(t, err)
//
//		_, err = w.Write(payload)
//		assert.NoError(t, err)
//	}))
//	defer ts.Close()
//
//	client := New(ts.URL)
//	actual, err := client.Prepare(context.Background(), expected)
//	assert.Nil(t, err)
//
//	assert.Equal(t, expected, *actual)
//}

//func TestSubmit(t *testing.T) {
//	expected := AttestedTransaction{
//		Transaction: Transaction{
//			IdempotencyKey: uuid.NewV4(),
//			Amount:         decimal.New(1, 0),
//			To:             uuid.NewV4(),
//		},
//		DocumentID:          testutils.RandomString(),
//		Version:             testutils.RandomString(),
//		State:               testutils.RandomString(),
//		AttestationDocument: base64.StdEncoding.EncodeToString([]byte(testutils.RandomString())),
//	}
//
//	buf := new(bytes.Buffer)
//	err := json.NewEncoder(buf).Encode(expected)
//	require.NoError(t, err)
//
//	headers := AuthorizationHeaders{
//		"Host":           testutils.RandomString(),
//		"Digest":         testutils.RandomString(),
//		"Signature":      testutils.RandomString(),
//		"Content-Length": strconv.Itoa(buf.Len()),
//		"Content-Type":   testutils.RandomString(),
//	}
//
//	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		require.Equal(t, http.MethodPost, r.Method)
//		require.Equal(t, "/v1/payments/submit", r.URL.Path)
//
//		// check headers
//		assert.Equal(t, headers["Host"], r.Host)
//		assert.Equal(t, headers["Digest"], r.Header.Get("Digest"))
//		assert.Equal(t, headers["Signature"], r.Header.Get("Signature"))
//		assert.Equal(t, headers["Content-Length"], r.Header.Get("Content-Length"))
//		assert.Equal(t, headers["Content-Type"], r.Header.Get("Content-Type"))
//
//		// assert we received the expected attested transaction
//		var attestedTransaction AttestedTransaction
//		err := json.NewDecoder(r.Body).Decode(&attestedTransaction)
//
//		require.NoError(t, err)
//		assert.Equal(t, expected, attestedTransaction)
//
//		// return the received transaction
//		w.WriteHeader(http.StatusCreated)
//	}))
//	defer ts.Close()
//
//	client := New(ts.URL)
//	err = client.Submit(context.Background(), expected, headers)
//	assert.Nil(t, err)
//}
