package payments

type KeyStatementPrincipal struct {
	AWS interface{}
}

type KeyStatement struct {
	Sid       string
	Effect    string
	Principal KeyStatementPrincipal
	Action    interface{}
	Condition map[string]interface{}
}

type KeyPolicy struct {
	Version   *string
	Statement []KeyStatement
}
