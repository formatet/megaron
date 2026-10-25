// Player names shared by city rows, construction dispatches and their actions.
export const BUILDING_LABELS={farm:'Farm',barracks:'Barracks',mine:'Mine',lumbermill:'Lumbermill',stonequarry:'Stone Quarry',market:'Market',wall:'Wall',tower:'Tower',harbour:'Harbour',shipyard:'Shipyard',foundry:'Foundry',stable:'Stable',bronze_wall:'Bronze Wall',olive_press:'Olive Press',winery:'Winery',temple:'Temple',pen:'Pen',silo:'Granary'};
export const buildingLabel=(key)=>BUILDING_LABELS[key]||'Building';
