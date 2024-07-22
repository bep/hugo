import { readInput, writeOutput } from './common';
import katex from 'katex';

const render = function (input) {
	const expression = input.expression;
	const id = input.id;
	delete input.expression;
	delete input.id;
	input.throwOnError = false;
	writeOutput({ id: id, output: katex.renderToString(expression, input) });
};

(() => {
	readInput(render);
})();
