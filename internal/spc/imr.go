// Package spc implementa Statistical Process Control sobre métricas de
// pipelines de CI/CD: carta Individuals/Moving-Range (I-MR) e regras de
// Nelson para detecção de causa especial.
package spc

import (
	"fmt"
	"time"
)

// Point é uma observação individual da série temporal de uma métrica de
// pipeline (por exemplo, duração de um build).
type Point struct {
	Timestamp time.Time
	Value     float64
}

// IMRPoint é um Point enriquecido com o resultado do cálculo da carta I-MR.
type IMRPoint struct {
	Point
	MovingRange  float64 // |valor atual - valor anterior|; 0 no primeiro ponto.
	OutOfControl bool    // true se Value está fora de [LCL, UCL].
}

// IMRChart é o resultado do cálculo de uma carta Individuals/Moving-Range.
//
// A carta I-MR é o padrão do KaizenOps (ver ADR-1): builds chegam um a um
// (n=1 por observação), não em subgrupos racionais, portanto X-bar/R não se
// aplica.
type IMRChart struct {
	CenterLine float64 // média dos Value.
	MRBar      float64 // média das moving ranges (a partir do 2º ponto).
	UCL        float64 // CenterLine + 2.66*MRBar.
	LCL        float64 // CenterLine - 2.66*MRBar.
	MRUCL      float64 // 3.267*MRBar (limite superior da carta de moving range).
	Points     []IMRPoint
}

// Constantes clássicas de controle para carta I-MR (subgrupo de n=2 na carta
// de moving range): d2=1.128 -> 3/d2 = 2.66; D4=3.267 para o limite superior
// da moving range.
const (
	imrSigmaConstant = 2.66
	mrUCLConstant    = 3.267
)

// ComputeIMR calcula a carta Individuals/Moving-Range a partir da série de
// pontos fornecida. Retorna erro se houver menos de 2 pontos, pois não é
// possível estimar MRBar (média das moving ranges) com uma única observação.
func ComputeIMR(points []Point) (IMRChart, error) {
	if len(points) < 2 {
		return IMRChart{}, fmt.Errorf("computing IMR chart: need at least 2 points, got %d", len(points))
	}

	var sum float64
	for _, p := range points {
		sum += p.Value
	}
	centerLine := sum / float64(len(points))

	imrPoints := make([]IMRPoint, len(points))
	var mrSum float64
	for i, p := range points {
		var mr float64
		if i > 0 {
			mr = p.Value - points[i-1].Value
			if mr < 0 {
				mr = -mr
			}
			mrSum += mr
		}
		imrPoints[i] = IMRPoint{
			Point:       p,
			MovingRange: mr,
		}
	}
	mrBar := mrSum / float64(len(points)-1)

	ucl := centerLine + imrSigmaConstant*mrBar
	lcl := centerLine - imrSigmaConstant*mrBar
	mrUCL := mrUCLConstant * mrBar

	for i := range imrPoints {
		v := imrPoints[i].Value
		imrPoints[i].OutOfControl = v > ucl || v < lcl
	}

	return IMRChart{
		CenterLine: centerLine,
		MRBar:      mrBar,
		UCL:        ucl,
		LCL:        lcl,
		MRUCL:      mrUCL,
		Points:     imrPoints,
	}, nil
}
