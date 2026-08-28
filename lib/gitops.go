package lib

// GitHub's noreply format (<bot-user-id>+<app-slug>[bot]@users.noreply.github.com)
// is what links commits to the app account; id from api.github.com/users/ekko-github%5Bbot%5D
var user = "ekko-github[bot]"
var mail = "285673000+ekko-github[bot]@users.noreply.github.com"

// FLUXCD
// The question of which method to use to update the yaml file, sb uses YQ
// And how does  the namespace get fed into the action?
