// Package schema: общие хелперы для режима генерации (mode) и required-логики.
// Используются StructRenderer и SetDefaultsRenderer — оба читают текущий режим
// typeMapper'а и определяют required поля по mode + USE_REQUIRED_V2.
//
// Режим хранится в typeMapper'е (устанавливается через TypeMapper.SetMode).
// Renderer'ы не хранят локальное состояние режима — walker вызывает OnSplitStruct
// на каждом renderer'е последовательно, и StructRenderer может оставить режим
// в modeRequest/modeResponse. SetDefaultsRenderer обязан выставить режим сам
// перед рендером (см. SetDefaultsRenderer.OnStruct/OnSplitStruct).
package schema

import (
	"nschugorev/oapigenerator/internal/generator/render"
	"nschugorev/oapigenerator/internal/parser"
)

// currentMode извлекает текущий режим typeMapper'а. Renderer'ы не хранят
// режим отдельно — он выставляется через TypeMapper.SetMode и читается
// обратно через modeGetter.
//
// Возвращает "", если typeMapper не реализует modeGetter (тестовые fakes
// могут опускать).
func currentMode(ctx *render.RenderContext) string {
	if ctx == nil {
		return ""
	}

	if mg, ok := ctx.TypeMapper.(modeGetter); ok {
		return mg.Mode()
	}

	return ""
}

// modeGetter — optional-интерфейс для чтения текущего режима typeMapper'а.
// typeMapperAdapter реализует его; тестовые fakes могут опускать.
type modeGetter interface {
	Mode() string
}

// requiredForMode возвращает, является ли поле required в текущем режиме
// генерации. Логика зависит от флага USE_REQUIRED_V2 (см. комментарий к
// generator.requiredForMode — там же SSOT). Режим читается через
// TypeMapper.SetMode — renderer'ы хранят режим в typeMapper'е, не локально.
//
// При USE_REQUIRED_V2 on:
//   - modeRequest → p.RequestRequired;
//   - modeResponse → p.ResponseRequired;
//   - "" (моно) → если поле есть в x-* списках, required только если в обоих;
//     иначе fallback на OAS required.
//
// При USE_REQUIRED_V2 off — fallback на p.Required.
func requiredForMode(ctx *render.RenderContext, p *parser.Property) bool {
	if ctx == nil || !ctx.Features.UseRequiredV2.Value {
		return p.Required
	}

	switch currentMode(ctx) {
	case modeRequest:
		return p.RequestRequired
	case modeResponse:
		return p.ResponseRequired
	default:
		// Моно-режим при v2 on: если поле есть в x-* списках, required
		// только если в обоих; иначе fallback на OAS required.
		if p.RequestRequired || p.ResponseRequired {
			return p.RequestRequired && p.ResponseRequired
		}

		return p.Required
	}
}
