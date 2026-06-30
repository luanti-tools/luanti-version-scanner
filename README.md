# Luanti Version Scanner

This is a linux cli program designed with the goal of scanning through a Luanti
mod/modpack/game, and finding the min and max versions need to run it. It is
able to write the data to the .conf files of the scanned mod/modpack/game. It
does have limits though, use your good judgement.

## Features

* Database 550+ known Luanti api's, almost every api can be accounted for.
* Ability to determine if a api is guarded by a version check.
* Write findings to .conf files.
* Scan entire modpacks and games at a time.
* List every api found and checked.
* Determine min version all the way back to v0.4.0.
* Find max version based off of deprecated api's.

## Shortcomings

If you depend on a mod that requires a higher version of Luanti then your own
by it self, then this program will output a lower min version. The difficulties
in making this tool such that it can account for this are too great. It is
recommended that you check the min version required by all your dependencies to
find the real min version.

Also it is possible for this mod state that the max version is lower then the
min. This is caused by using an api that was deprecated before the newest api
used. The solution is to update the deprecated api to it's newer counterpart.
In this way you can use this tool to find depreciated api's, but it will not
tell you the new counterpart (if it has one).

There is the possibility that some valid api's have been missed, in that case
please open an issue and we will attempt to add it to the database. There are
some undocumented api's that will not be added to this tool, since they are not
documented there is no way of knowing when they were added, or removed without
checking every commit, which would be very tedious.

## Install

Place `luanti-version-scan` inside your `/bin` directory of choice.

## License

Everything is licensed under [MIT](https://choosealicense.com/licenses/mit/).
