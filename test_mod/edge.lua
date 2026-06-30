-- Test edge cases

-- Feature check inside a comment (should be IGNORED):
-- if minetest.features.glasslike_framed then

-- Feature check inside a string (should be IGNORED):
local s = "if minetest.features.glasslike_framed then"

-- Feature check inside a long string (should be IGNORED):
local ls = [[
if minetest.features.glasslike_framed then
]]

-- Real feature check (should be DETECTED):
if minetest.features.use_texture_alpha then
	minetest.register_node("test:node", {
		description = "Test",
		tiles = {"test.png"},
	})
end

-- Nested blocks
if minetest.features.biome_weights then
	for i = 1, 10 do
		minetest.register_biome({
			name = "test_biome_" .. i,
		})
	end
else
	repeat
		-- fallback code
	until true
end

-- Version check
if minetest.get_version() then
	-- some version-dependent code
end

-- Multiple feature checks on same line
local a = minetest.features.glasslike_framed and minetest.features.direct_velocity_on_players
