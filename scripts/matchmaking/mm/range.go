package mm

var UNIQUE_RANGES = []string{
	"<1K",
	"1K-5K",
	"5K-10K",
	"10K-50K",
	"50K-100K",
	"100K+",
	"<5K",
	"5K-20K",
	"20K-50K",
	"100K-1M",
	"1M+",
	"5K-15K",
	"15K-50K",
	"50K+",
}

func RangeToMidpoint(val string) int {
	switch val {
	case "<1K":
		return 500
	case "1K-5K":
		return 3000
	case "5K-10K":
		return 7500
	case "10K-50K":
		return 30000
	case "50K-100K":
		return 75000
	case "100K+":
		return 150000
	case "<5K":
		return 2500
	case "5K-20K":
		return 12500
	case "20K-50K":
		return 35000
	case "100K-1M":
		return 550000
	case "1M+":
		return 1500000
	case "5K-15K":
		return 10000
	case "15K-50K":
		return 30000
	case "50K+":
		return 75000
	default:
		return 0
	}
}

// <Select
// 	items={[
// 		{ label: 'Less than 1,000', value: '<1K' },
// 		{ label: '1,000 - 5,000', value: '1K-5K' },
// 		{ label: '5,000 - 10,000', value: '5K-10K' },
// 		{ label: '10,000 - 50,000', value: '10K-50K' },
// 		{ label: '50,000 - 100,000', value: '50K-100K' },
// 		{ label: '100,000+', value: '100K+' },
// 	]}
// 	selectItemIcon={true}
// 	value={followerRange}
// 	onSelect={(val) => setFollowerRange(val)}
// />
// <Text style={[styles.label, { color: Colors(theme).text }]}>Monthly View Count</Text>
// <Select
// 	items={[
// 		{ label: 'Less than 5,000', value: '<5K' },
// 		{ label: '5,000 - 20,000', value: '5K-20K' },
// 		{ label: '20,000 - 50,000', value: '20K-50K' },
// 		{ label: '50,000 - 100,000', value: '50K-100K' },
// 		{ label: '100,000 - 1M', value: '100K-1M' },
// 		{ label: '1M+', value: '1M+' },
// 	]}
// 	selectItemIcon={true}
// 	value={monthlyViews}
// 	onSelect={(val) => setMonthlyViews(val)}
// />
// <Text style={[styles.label, { color: Colors(theme).text }]}>Monthly Interaction Count</Text>
// <Select
// 	items={[
// 		{ label: 'Less than 1,000', value: '<1K' },
// 		{ label: '1,000 - 5,000', value: '1K-5K' },
// 		{ label: '5,000 - 15,000', value: '5K-15K' },
// 		{ label: '15,000 - 50,000', value: '15K-50K' },
// 		{ label: '50,000+', value: '50K+' },
// 	]}
// 	selectItemIcon={true}
// 	value={monthlyInteractions}
// 	onSelect={(val) => setMonthlyInteractions(val)}
// />
