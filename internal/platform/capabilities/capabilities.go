package capabilities

import "context"

const (
	FeatureAnalyze = "analyze"
	FeatureBuild   = "build"
	FeatureAuth    = "auth"
	FeatureEvents  = "events"
)

type Set map[string]bool

type Provider interface {
	List(ctx context.Context) (Set, error)
	Has(ctx context.Context, feature string) (bool, error)
}

type AllAccessProvider struct{}

func NewAllAccessProvider() *AllAccessProvider {
	return &AllAccessProvider{}
}

func (p *AllAccessProvider) List(_ context.Context) (Set, error) {
	return Set{
		FeatureAnalyze: true,
		FeatureBuild:   true,
		FeatureAuth:    true,
		FeatureEvents:  true,
	}, nil
}

func (p *AllAccessProvider) Has(ctx context.Context, feature string) (bool, error) {
	features, err := p.List(ctx)
	if err != nil {
		return false, err
	}
	allowed, ok := features[feature]
	if !ok {
		return false, nil
	}
	return allowed, nil
}
