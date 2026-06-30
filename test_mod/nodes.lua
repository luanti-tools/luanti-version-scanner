-- nodes.lua - additional node definitions with unknown API usage

minetest.register_node("test_mod:fancy_node", {
	description = "Fancy Node",
	tiles = {"test_mod_fancy.png"},
	groups = {choppy = 2},
})

-- Unguarded 5.8.0 API call
local clouds = minetest.get_clouds()

-- An unknown API call (not in database)
minetest.some_unknown_function()

-- Method call on a VoxelManip
local vm = minetest.get_voxel_manip()
vm:read_from_map({x=0,y=0,z=0}, {x=10,y=10,z=10})
