import { readInput, writeOutput } from './common';

const greet = function (input) {
	writeOutput({ id: input.id, greeting: 'Hello ' + input.name + '!' });
};

(() => {
	readInput(greet);
})();
