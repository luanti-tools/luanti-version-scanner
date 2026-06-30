-- test_mod/init.lua
-- A test mod with various API usage patterns

-- Unguarded base API calls (require 5.0.0)
minetest.register_node("test_mod:my_node", {
	description = "My Node",
	tiles = {"test_mod_node.png"},
	groups = {cracky = 3},
})

minetest.register_craft({
	output = "test_mod:my_node",
	recipe = {{"default:stone", "default:stone"}},
})

-- Unguarded 5.3.0 API call
local pos = {x = 0, y = 0, z = 0}
local nearby = minetest.find_node_near(pos, 10, "group:stone")

-- Guarded 5.8.0 API call with feature check
if minetest.features.glasslike_framed then
	-- This is guarded by the feature check
	local sky = minetest.get_sky()
else
	-- Fallback code for older versions
end

-- Feature flag check (defines minimum for this feature)
local has_alpha = minetest.features.use_texture_alpha

-- Method calls on objects
minetest.register_on_joinplayer(function(player)
	local name = player:get_player_name()
	player:set_hp(20)
end)

-- Nested if with feature check
if minetest.features.biome_weights then
	minetest.register_biome({
		name = "test_biome",
	})
end

-- Version check guard (not a feature check)
if minetest.get_version() then
	-- version-dependent code
end

-- Deeply nested guarded call
if minetest.features.direct_velocity_on_players then
	if something then
		local biome = minetest.get_biome_data(pos)
	end
end
