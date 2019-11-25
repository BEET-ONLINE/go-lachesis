package poset

import (
	"math/big"
	"time"

	"github.com/Fantom-foundation/go-ethereum/common"

	"github.com/Fantom-foundation/go-lachesis/inter"
)

func mul(a *big.Int, b uint64) {
	a.Mul(a, new(big.Int).SetUint64(b))
}

func div(a *big.Int, b uint64) {
	a.Div(a, new(big.Int).SetUint64(b))
}

func sub(a *big.Int, b uint64) {
	a.Sub(a, new(big.Int).SetUint64(b))
}

func (p *Poset) calcValidatorGasPowerPerH(validator common.Address) (perHour, maxGasPower, startup uint64) {
	stake := p.Validators.Get(validator)
	if stake == 0 {
		return 0, 0, 0
	}

	gas := p.dag.GasPower

	validatorGasPowerPerH_bn := new(big.Int).SetUint64(gas.TotalPerH)
	mul(validatorGasPowerPerH_bn, uint64(stake))
	div(validatorGasPowerPerH_bn, uint64(p.Validators.TotalStake()))
	perHour = validatorGasPowerPerH_bn.Uint64()

	validatorMaxGasPower_bn := new(big.Int).Set(validatorGasPowerPerH_bn)
	mul(validatorMaxGasPower_bn, uint64(gas.MaxGasPowerPeriod))
	div(validatorMaxGasPower_bn, uint64(time.Hour))
	maxGasPower = validatorMaxGasPower_bn.Uint64()

	validatorStartup_bn := new(big.Int).Set(validatorGasPowerPerH_bn)
	mul(validatorStartup_bn, uint64(gas.StartupPeriod))
	div(validatorStartup_bn, uint64(time.Hour))
	startup = validatorStartup_bn.Uint64()
	if startup < gas.MinStartupGasPower {
		startup = gas.MinStartupGasPower
	}

	return
}

// CalcGasPower calculates available gas power for the event, i.e. how many gas its content may consume
func (p *Poset) CalcGasPower(e *inter.EventHeaderData) uint64 {
	gasPowerPerH, maxGasPower, startup := p.calcValidatorGasPowerPerH(e.Creator)

	var prevGasPowerLeft uint64

	if e.SelfParent() != nil {
		selfParent := p.GetEventHeader(e.Epoch, *e.SelfParent())
		prevGasPowerLeft = selfParent.GasPowerLeft
	} else if prevConfirmedHeader := p.PrevEpoch.LastHeaders[e.Creator]; prevConfirmedHeader != nil {
		prevGasPowerLeft = prevConfirmedHeader.GasPowerLeft
		if prevGasPowerLeft < startup {
			prevGasPowerLeft = startup
		}
	} else {
		prevGasPowerLeft = startup
	}

	// TODO
	gasPowerAllocated_bn := new(big.Int).SetUint64(uint64(1))
	mul(gasPowerAllocated_bn, gasPowerPerH)
	div(gasPowerAllocated_bn, uint64(time.Hour))

	gasPower := gasPowerAllocated_bn.Uint64() + prevGasPowerLeft
	if gasPower > maxGasPower {
		gasPower = maxGasPower
	}

	return gasPower
}
