package service

import (
	"embed"
	"errors"
)

//go:embed mailtemplates/*.html
var mailTemplateFS embed.FS

// errUnknownTemplate คืนเมื่อ alias ไม่ตรงกับ template ที่ embed ไว้
var errUnknownTemplate = errors.New("unknown email template alias")

type mailTemplate struct {
	Subject string
	Html    string
}

// mailTemplates คือ template ที่ compile มากับ binary ทั้งหมด
// แก้ไขต้องแก้ไฟล์ html แล้ว rebuild (ไม่มีการแก้ตอนรันแล้ว)
var mailTemplates = map[string]mailTemplate{
	"oryca.verify-email": {
		Subject: "Verify your email address",
		Html:    mustReadMailTemplate("verify-email.html"),
	},
	"oryca.set-password": {
		Subject: "Set your password",
		Html:    mustReadMailTemplate("set-password.html"),
	},
}

func mustReadMailTemplate(name string) string {
	b, err := mailTemplateFS.ReadFile("mailtemplates/" + name)
	if err != nil {
		panic("mail: read embedded template " + name + ": " + err.Error())
	}
	return string(b)
}

func getMailTemplate(alias string) (mailTemplate, error) {
	tmpl, ok := mailTemplates[alias]
	if !ok {
		return mailTemplate{}, errUnknownTemplate
	}
	return tmpl, nil
}
