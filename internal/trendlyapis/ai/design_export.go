package ai

// What the Design Studio needs from this package, and nothing more.
//
// The studio authors HTML scenes, so it needs the same two things the chat
// surface gives the model when it designs: the CRAFT CONTRACT (the floor every
// design is held to — margins, type sizes, what the renderer supports) and the
// BRAND'S OWN design system (palette, fonts, voice, per-brand rules).
//
// Exported as one call rather than by exposing the internals, because the two
// are only ever useful together and their order matters: the craft contract is
// the floor, the brand's system is the identity, and where they disagree the
// brand wins. A caller assembling them itself would eventually get that
// backwards and produce on-spec designs that look like nobody's brand.

// DesignContextFor returns the system-prompt block for authoring a design or a
// scene for this brand.
//
// Safe to call for a brand with no design system: the craft contract alone is
// still the right floor, and a brand that has not filled one in should get
// competent generic design rather than nothing.
func DesignContextFor(brandID string) string {
	block := designSystemPromptBlock(loadDesignSystem(brandID))
	return block + designCraftInstructions
}

// BrandKitCSS returns the brand's CSS custom properties, to be inlined at the
// top of a generated document so colours and fonts are declared once rather
// than repeated per element — and so a later edit can change the brand's look
// without rewriting every rule.
func BrandKitCSS(brandID, fallbackLogo string) string {
	return brandKitCSS(loadDesignSystem(brandID), fallbackLogo)
}
